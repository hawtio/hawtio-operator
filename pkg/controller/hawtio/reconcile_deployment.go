package hawtio

import (
	"context"
	"fmt"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	appsv1 "k8s.io/api/apps/v1"
	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"github.com/hawtio/hawtio-operator/pkg/resources"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

func (r *ReconcileHawtio) reconcileDeployment(ctx context.Context, hawtio *hawtiov2.Hawtio, deploymentConfig DeploymentConfiguration) (controllerutil.OperationResult, error) {
	// The object the K8s client fetches into, and ultimately saves.
	targetDeployment := resources.NewDefaultDeployment(hawtio)

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, targetDeployment, func() error {
		// A read-only copy of the cluster state for diff logging
		liveSnapshot := targetDeployment.DeepCopy()

		// Set the owner reference for garbage collection
		if err := controllerutil.SetControllerReference(hawtio, targetDeployment, r.scheme); err != nil {
			return err
		}

		clientCertSecretVersion := ""
		if deploymentConfig.clientCertSecret != nil {
			r.logger.V(util.DebugLogLevel).Info("Assigning to deployment client certificate secret", "Resource Version", deploymentConfig.clientCertSecret.GetResourceVersion())
			clientCertSecretVersion = deploymentConfig.clientCertSecret.GetResourceVersion()
		}

		reqLogger := hawtioLogger.WithName(fmt.Sprintf("%s-reconcileDeployment", hawtio.Name))
		// Local, ideal state generated from the Hawtio CR
		blueprint, err := resources.NewDeployment(hawtio, r.apiSpec,
			deploymentConfig.openShiftConsoleURL,
			deploymentConfig.configMap.GetResourceVersion(),
			clientCertSecretVersion,
			r.BuildVariables, reqLogger)
		if err != nil {
			reqLogger.Error(err, "Error reconciling deployment")
			return err
		}

		serverBlueprint, err := hydrateDefaults(ctx, r.client, blueprint, func(source, hydrated *appsv1.Deployment) {
			// If hydration stripped required fields, patch them directly back from source
			if hydrated.Spec.Selector == nil {
				hydrated.Spec.Selector = source.Spec.Selector
			}
			if len(hydrated.Spec.Template.Spec.Containers) == 0 {
				hydrated.Spec.Template.Spec.Containers = source.Spec.Template.Spec.Containers
			}
		})
		if err != nil {
			return err
		}

		targetDeployment.Labels = util.MergeMap(targetDeployment.Labels, blueprint.Labels)
		targetDeployment.Annotations = util.MergeMap(targetDeployment.Annotations, blueprint.Annotations)
		// Assign the fully hydrated and patched blueprint spec
		targetDeployment.Spec = serverBlueprint.Spec

		// Report any known differences to the log (only if in debug log level)
		util.ReportDiff("Deployment", liveSnapshot, targetDeployment)

		return nil
	})

	// There is a resource but the default client cache cannot see it
	// since it is legacy and not labelled. The create failed so it
	// should be adopted and reconciled.
	if err != nil && kerrors.IsAlreadyExists(err) {
		adoptionErr := r.adoptLegacyResource(ctx, resources.NewDefaultDeployment(hawtio))
		if adoptionErr != nil {
			// If adoption failed (e.g., API error) or an adoption was called
			// return any of these errors
			return controllerutil.OperationResultNone, adoptionErr
		}
	}

	util.ReportResourceChange("Deployment", targetDeployment, opResult)
	return opResult, err
}
