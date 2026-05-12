package hawtio

import (
	"context"
	"fmt"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	oauthv1 "github.com/openshift/api/oauth/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"

	"github.com/hawtio/hawtio-operator/pkg/resources"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

func (r *ReconcileHawtio) verifyRBACConfigMap(ctx context.Context, hawtio *hawtiov2.Hawtio, namespacedName client.ObjectKey) (bool, error) {
	cm := hawtio.Spec.RBAC.ConfigMap
	if cm == "" {
		return true, nil // No RBAC configMap specified so default will be used
	}

	r.logger.V(util.DebugLogLevel).Info("Checking Hawtio.Spec.RBAC config map is valid")

	// Check that the ConfigMap exists
	var rbacConfigMap corev1.ConfigMap

	// Use API Reader to bypass cache and obtain legacy resource
	err := r.apiReader.Get(ctx, types.NamespacedName{Namespace: namespacedName.Namespace, Name: cm}, &rbacConfigMap)
	if err != nil {
		r.logger.Error(err, "Failed to get RBAC ConfigMap")
		return false, err
	}

	if _, ok := rbacConfigMap.Data[resources.RBACConfigMapKey]; !ok {
		r.logger.Info("RBAC ConfigMap does not contain expected key: "+resources.RBACConfigMapKey, "ConfigMap", cm)
		// Let's poll for the RBAC ConfigMap to contain the expected key
		return false, nil
	}

	return true, nil
}

func (r *ReconcileHawtio) reconcileServiceAccount(ctx context.Context, hawtio *hawtiov2.Hawtio) (controllerutil.OperationResult, error) {
	serviceAccount := resources.NewDefaultServiceAccount(hawtio)

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, serviceAccount, func() error {
		// A read-only copy of the cluster state for diff logging
		liveSnapshot := serviceAccount.DeepCopy()

		// Set the owner reference for garbage collection.
		if err := controllerutil.SetControllerReference(hawtio, serviceAccount, r.scheme); err != nil {
			return err
		}

		reqLogger := hawtioLogger.WithName(fmt.Sprintf("%s-reconcileServiceAccount", hawtio.Name))
		crServiceAccount, err := resources.NewServiceAccount(hawtio, reqLogger)
		if err != nil {
			return err
		}

		serviceAccount.Labels = util.MergeMap(serviceAccount.Labels, crServiceAccount.GetLabels())
		serviceAccount.Annotations = util.MergeMap(serviceAccount.Annotations, crServiceAccount.GetAnnotations())

		// Report any known differences to the log (only if in debug log level)
		util.ReportDiff("ServiceAccount", liveSnapshot, serviceAccount)

		return nil
	})

	// There is a resource but the default client cache cannot see it
	// since it is legacy and not labelled. The create failed so it
	// should be adopted and reconciled.
	if err != nil && kerrors.IsAlreadyExists(err) {
		adoptionErr := r.adoptLegacyResource(ctx, resources.NewDefaultServiceAccount(hawtio))
		if adoptionErr != nil {
			// If adoption failed (e.g., API error) or an adoption was called
			// return any of these errors
			return controllerutil.OperationResultNone, adoptionErr
		}
	}

	if err != nil {
		return opResult, err
	}

	util.ReportResourceChange("ServiceAccount", serviceAccount, opResult)
	return opResult, nil
}

func (r *ReconcileHawtio) reconcileOAuthClient(ctx context.Context, hawtio *hawtiov2.Hawtio, newRouteURL string, namespacedName client.ObjectKey) (controllerutil.OperationResult, error) {
	if !r.apiSpec.IsOpenShift4 {
		// Not applicable to cluster
		return controllerutil.OperationResultNone, nil
	}

	clientName := namespacedName.Name + "-" + namespacedName.Namespace
	shouldExist := hawtio.Spec.Type == hawtiov2.ClusterHawtioDeploymentType

	// Should the OAuthClient exist at all?
	if !shouldExist {
		// The CR is not cluster-scoped, so we must ensure the OAuthClient is cleaned up.
		// Note: We use the direct client here to handle potential permission issues.
		existingOAuthClient := &oauthv1.OAuthClient{}
		err := r.client.Get(ctx, types.NamespacedName{Name: clientName}, existingOAuthClient)
		if err != nil {
			if kerrors.IsNotFound(err) {
				return controllerutil.OperationResultNone, nil // Already gone, which is correct.
			}
			// If we get a Forbidden error, we assume we can't manage it anyway.
			if kerrors.IsForbidden(err) {
				r.logger.Info(fmt.Sprintf("Operator is not permitted to clean up cluster OAuthClient %v; skipping.", namespacedName))
				return controllerutil.OperationResultNone, nil
			}
			return controllerutil.OperationResultNone, err
		}

		// Found an existing OAuthClient, let's remove our URI from it.
		r.logger.Info(fmt.Sprintf("Hawtio is not cluster-scoped, removing RedirectURI from OAuthClient %v", namespacedName))
		if resources.RemoveRedirectURIFromOauthClient(existingOAuthClient, newRouteURL) {
			err := r.client.Update(ctx, existingOAuthClient)
			return controllerutil.OperationResultUpdated, err
		}

		return controllerutil.OperationResultNone, nil
	}

	// Ensure the OAuthClient exists and is correctly configured.
	targetOAuthClient := resources.NewDefaultOAuthClient(clientName)

	// We use CreateOrUpdate to ensure the base object exists.
	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, targetOAuthClient, func() error {
		// A read-only copy of the cluster state for diff logging
		liveSnapshot := targetOAuthClient.DeepCopy()

		reqLogger := hawtioLogger.WithName(fmt.Sprintf("%s-%s-reconcileOAuthClient", namespacedName.Name, namespacedName.Namespace))
		blueprint := resources.NewOAuthClient(clientName, hawtio, reqLogger)

		serverBlueprint, err := hydrateDefaults(ctx, r.client, blueprint, func(source, hydrated *oauthv1.OAuthClient) {
			// If hydration stripped required fields, patch them directly back from source
			if hydrated.GrantMethod == "" {
				hydrated.GrantMethod = source.GrantMethod
			}
			if len(hydrated.RedirectURIs) == 0 && len(source.RedirectURIs) > 0 {
				hydrated.RedirectURIs = source.RedirectURIs
			}
		})
		if err != nil {
			return err
		}

		targetOAuthClient.GrantMethod = serverBlueprint.GrantMethod
		targetOAuthClient.Labels = util.MergeMap(targetOAuthClient.Labels, blueprint.Labels)

		// Only set RedirectURIs if the list is nil (creation time).
		// Never overwrite it if it exists, because we manage specific entries below.
		if targetOAuthClient.RedirectURIs == nil {
			targetOAuthClient.RedirectURIs = serverBlueprint.RedirectURIs
		}

		// Report any known differences to the log (only if in debug log level)
		util.ReportDiff("OAuthClient", liveSnapshot, targetOAuthClient)

		return nil
	})

	// There is a resource but the default client cache cannot see it
	// since it is legacy and not labelled. The create failed so it
	// should be adopted and reconciled.
	if err != nil && kerrors.IsAlreadyExists(err) {
		adoptionErr := r.adoptLegacyResource(ctx, resources.NewDefaultOAuthClient(clientName))
		if adoptionErr != nil {
			// If adoption failed (e.g., API error) or an adoption was called
			// return any of these errors
			return controllerutil.OperationResultNone, adoptionErr
		}
	}

	if err != nil {
		return controllerutil.OperationResultNone, err
	}

	// Read-Modify-Write for RedirectURIs
	// We must re-fetch the object to ensure we have the latest version
	// before modifying the list.
	err = r.client.Get(ctx, types.NamespacedName{Name: clientName}, targetOAuthClient)
	if err != nil {
		return controllerutil.OperationResultNone, err
	}

	updateOAuthClient := false
	oldRouteURL := hawtio.Status.URL
	// Remove the old URL if it's different from the new one
	if oldRouteURL != "" && oldRouteURL != newRouteURL {
		r.logger.Info("Removing stale RedirectURI from OAuthClient", "URI", oldRouteURL)
		if resources.RemoveRedirectURIFromOauthClient(targetOAuthClient, oldRouteURL) {
			updateOAuthClient = true
		}
	}

	// Add the current route URL if it's not already present.
	if ok, _ := resources.OauthClientContainsRedirectURI(targetOAuthClient, newRouteURL); !ok && newRouteURL != "" {
		r.logger.V(util.DebugLogLevel).Info("OAuthClient URI mismatch detected",
			"Wanted", newRouteURL,
			"ExistingURIs", targetOAuthClient.RedirectURIs)

		r.logger.Info("Adding new RedirectURI to OAuthClient", "URI", newRouteURL)
		targetOAuthClient.RedirectURIs = append(targetOAuthClient.RedirectURIs, newRouteURL)
		updateOAuthClient = true
	}

	if updateOAuthClient {
		err := r.client.Update(ctx, targetOAuthClient)
		return controllerutil.OperationResultUpdated, err
	}

	util.ReportResourceChange("OAuthClient", targetOAuthClient, opResult)
	return opResult, nil
}
