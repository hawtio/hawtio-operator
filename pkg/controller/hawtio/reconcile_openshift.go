package hawtio

import (
	"context"
	"fmt"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	consolev1 "github.com/openshift/api/console/v1"
	routev1 "github.com/openshift/api/route/v1"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"

	"github.com/hawtio/hawtio-operator/pkg/openshift"
	"github.com/hawtio/hawtio-operator/pkg/resources"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

func (r *ReconcileHawtio) removeConsoleLink(ctx context.Context, consoleLinkName string) (controllerutil.OperationResult, error) {
	consoleLink := &consolev1.ConsoleLink{}
	err := r.client.Get(ctx, types.NamespacedName{Name: consoleLinkName}, consoleLink)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// The link doesn't exist, which is the source state. Nothing to do.
			return controllerutil.OperationResultNone, nil
		}
		// A real error occurred trying to get the object.
		return controllerutil.OperationResultNone, err
	}

	// If we get here, we found a stale ConsoleLink that needs to be deleted.
	r.logger.Info("Deleting stale ConsoleLink", "ConsoleLink.Name", consoleLinkName)
	if err := r.client.Delete(ctx, consoleLink); err != nil {
		return controllerutil.OperationResultNone, err
	}

	return controllerutil.OperationResultUpdated, nil
}

func (r *ReconcileHawtio) reconcileConsoleLink(ctx context.Context, hawtio *hawtiov2.Hawtio, namespacedName client.ObjectKey, deploymentConfig DeploymentConfiguration, route *routev1.Route) (controllerutil.OperationResult, error) {
	// If not OpenShift 4, ConsoleLink is irrelevant. Do nothing.
	if !r.apiSpec.IsOpenShift4 {
		r.logger.V(util.DebugLogLevel).Info("Not an OpenShift 4 cluster, skipping ConsoleLink reconciliation.")
		return controllerutil.OperationResultNone, nil
	}

	consoleLinkName := namespacedName.Name + "-" + namespacedName.Namespace

	// The only prerequisites are being on OCP and having a valid Route.
	validRoute := r.apiSpec.Routes && route != nil && route.Spec.Host != ""
	isClusterType := hawtio.Spec.Type == hawtiov2.ClusterHawtioDeploymentType
	isNamespaceTypeWithDashboard := hawtio.Spec.Type == hawtiov2.NamespaceHawtioDeploymentType && r.apiSpec.IsOpenShift43Plus
	shouldExist := validRoute && (isClusterType || isNamespaceTypeWithDashboard)

	// Prerequisite check
	r.logger.V(util.DebugLogLevel).Info("Reconcile ConsoleLink - Prerequisite Check")
	if !shouldExist {
		r.logger.V(util.DebugLogLevel).Info("Removing ConsoleLink as not applicable", "valid route", validRoute, "cluster link?", isClusterType, "namespace link?", isNamespaceTypeWithDashboard)
		return r.removeConsoleLink(ctx, consoleLinkName)
	}

	r.logger.V(util.DebugLogLevel).Info("Reconcile ConsoleLink - Retrieving HawtConfig")
	hawtconfig, err := resources.GetHawtioConfig(deploymentConfig.configMap)
	if err != nil {
		r.logger.Error(err, "Failed to get hawtconfig")
		return controllerutil.OperationResultNone, err
	}

	r.logger.V(util.DebugLogLevel).Info("Reconcile ConsoleLink - Creating new ConsoleLink")
	targetConsoleLink := openshift.NewDefaultConsoleLink(consoleLinkName)

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, targetConsoleLink, func() error {
		// A read-only copy of the cluster state for diff logging
		liveSnapshot := targetConsoleLink.DeepCopy()

		var blueprint *consolev1.ConsoleLink

		if hawtio.Spec.Type == hawtiov2.ClusterHawtioDeploymentType {
			r.logger.V(util.DebugLogLevel).Info("Adding console link as Application Menu Link")
			blueprint = openshift.NewApplicationMenuLink(consoleLinkName, namespacedName.Namespace, route, hawtconfig)
		} else if r.apiSpec.IsOpenShift43Plus {
			r.logger.V(util.DebugLogLevel).Info("Adding console link as Namespace Dashboard Link")
			blueprint = openshift.NewNamespaceDashboardLink(consoleLinkName, namespacedName.Namespace, route, hawtconfig)
		} else {
			// If no link should exist, we can't model that with CreateOrUpdate.
			return fmt.Errorf("Unsupported ConsoleLink configuration - neither Application nor Namespace link")
		}

		r.logger.V(util.DebugLogLevel).Info("ConsoleLink Default", "blueprint", blueprint)

		serverBlueprint, err := hydrateDefaults(ctx, r.client, blueprint, func(source, hydrated *consolev1.ConsoleLink) {
			r.logger.V(util.DebugLogLevel).Info("ConsoleLink Hydrating Callback", "hydrated", hydrated, "source", source)

			// If hydration stripped required fields, patch them directly back from source
			if hydrated.Spec.Href == "" || hydrated.Spec.Location == "" {
				r.logger.Info("ConsoleLink hydration dropped required fields. Restoring from source spec.",
					"ConsoleLink.Name", source.Name)

				hydrated.Spec.Href = source.Spec.Href
				hydrated.Spec.Location = source.Spec.Location
			}
		})

		r.logger.V(util.DebugLogLevel).Info("ConsoleLink Hydrated Default", "serverBlueprint", serverBlueprint)
		if err != nil {
			return err
		}

		r.logger.V(util.DebugLogLevel).Info("ConsoleLink Merging target with blueprint")
		targetConsoleLink.Labels = util.MergeMap(targetConsoleLink.Labels, blueprint.Labels)
		targetConsoleLink.Annotations = util.MergeMap(targetConsoleLink.Annotations, blueprint.Annotations)
		targetConsoleLink.Spec = serverBlueprint.Spec

		// Report any known differences to the log (only if in debug log level)
		util.ReportDiff("ConsoleLink", liveSnapshot, targetConsoleLink)

		return nil
	})

	// There is a resource but the default client cache cannot see it
	// since it is legacy and not labelled. The create failed so it
	// should be adopted and reconciled.
	if err != nil && kerrors.IsAlreadyExists(err) {
		adoptionErr := r.adoptLegacyResource(ctx, openshift.NewDefaultConsoleLink(consoleLinkName))
		if adoptionErr != nil {
			// If adoption failed (e.g., API error) or an adoption was called
			// return any of these errors
			return controllerutil.OperationResultNone, adoptionErr
		}
	}

	if err != nil {
		return opResult, err
	}

	util.ReportResourceChange("ConsoleLink", targetConsoleLink, opResult)
	return opResult, nil
}
