package hawtio

import (
	"context"
	"fmt"
	"strings"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/resources"
	kresources "github.com/hawtio/hawtio-operator/pkg/resources/kubernetes"
	oresources "github.com/hawtio/hawtio-operator/pkg/resources/openshift"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

func (r *ReconcileHawtio) reconcileService(ctx context.Context, hawtio *hawtiov2.Hawtio) (controllerutil.OperationResult, error) {
	targetService := resources.NewDefaultService(hawtio)

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, targetService, func() error {
		// A read-only copy of the cluster state for diff logging
		liveSnapshot := targetService.DeepCopy()
		oldClusterIP := targetService.Spec.ClusterIP
		oldClusterIPs := targetService.Spec.ClusterIPs

		// Set the owner reference for garbage collection.
		if err := controllerutil.SetControllerReference(hawtio, targetService, r.scheme); err != nil {
			return err
		}

		reqLogger := hawtioLogger.WithName(fmt.Sprintf("%s-reconcileService", hawtio.Name))
		blueprint := resources.NewService(hawtio, r.apiSpec, reqLogger)

		serverBlueprint, err := hydrateDefaults(ctx, r.client, blueprint, func(source, hydrated *corev1.Service) {
			// If hydration stripped required fields, patch them directly back from source
			if len(hydrated.Spec.Ports) == 0 {
				hydrated.Spec.Ports = source.Spec.Ports
			}
			if len(hydrated.Spec.Selector) == 0 {
				hydrated.Spec.Selector = source.Spec.Selector
			}

			for i := range hydrated.Spec.Ports {
				hydrated.Spec.Ports[i].NodePort = 0
			}

			// Ensure ClusterIP is not changed to a new random one from hydration of default.
			// If the service already exists, must preserve its IP.
			// If it doesn't exist (len == 0), leave it empty so K8s allocates a fresh one.
			if len(oldClusterIP) > 0 {
				hydrated.Spec.ClusterIP = oldClusterIP
				hydrated.Spec.ClusterIPs = oldClusterIPs
			} else {
				hydrated.Spec.ClusterIP = ""
				hydrated.Spec.ClusterIPs = nil
			}
		})
		if err != nil {
			return err
		}

		targetService.Labels = util.MergeMap(targetService.Labels, blueprint.Labels)
		targetService.Annotations = util.MergeMap(targetService.Annotations, serverBlueprint.Annotations)
		// Assign the fully hydrated and patched blueprint spec
		targetService.Spec = serverBlueprint.Spec

		// Report any known differences to the log (only if in debug log level)
		util.ReportDiff("Service", liveSnapshot, targetService)

		return nil
	})

	// There is a resource but the default client cache cannot see it
	// since it is legacy and not labelled. The create failed so it
	// should be adopted and reconciled.
	if err != nil && kerrors.IsAlreadyExists(err) {
		adoptionErr := r.adoptLegacyResource(ctx, resources.NewDefaultService(hawtio))
		if adoptionErr != nil {
			// If adoption failed (e.g., API error) or an adoption was called
			// return any of these errors
			return controllerutil.OperationResultNone, adoptionErr
		}
	}

	if err != nil {
		return opResult, err
	}

	util.ReportResourceChange("Service", targetService, opResult)
	return opResult, nil
}

func (r *ReconcileHawtio) reconcileRoute(ctx context.Context, hawtio *hawtiov2.Hawtio, deploymentConfig DeploymentConfiguration) (*routev1.Route, controllerutil.OperationResult, error) {
	// Only create a route if confirmed as Openshift and supports routes
	if !r.apiSpec.IsOpenShift4 || !r.apiSpec.Routes {
		return nil, controllerutil.OperationResultNone, nil
	}

	existingRoute := &routev1.Route{}
	err := r.client.Get(ctx, types.NamespacedName{Name: hawtio.Name, Namespace: hawtio.Namespace}, existingRoute)

	if err == nil {
		// A route was found. Now, apply the special condition check.
		isGenerated := strings.EqualFold(existingRoute.Annotations[oresources.RouteHostGeneratedAnnotation], "true")

		if hawtio.Spec.RouteHostName == "" && !isGenerated {
			// The user cleared the hostname, and the current route is not auto-generated.
			//
			// Emptying route host is ignored so it's not possible to re-generate the host
			// See https://github.com/openshift/origin/pull/9425
			// We must delete the route to force a regeneration.

			r.logger.Info("Deleting Route to trigger hostname regeneration.", "Route.Name", existingRoute.Name)
			if err := r.client.Delete(ctx, existingRoute); err != nil {
				r.logger.Error(err, "Failed to delete Route for regeneration")
				return nil, controllerutil.OperationResultNone, err
			}

			// Deletion was successful. We must stop this reconciliation loop here.
			// The next loop will find the Route is missing and will create a new one.
			// Returning (nil, nil) signals success for this loop, allowing the next one to proceed cleanly.
			return nil, controllerutil.OperationResultUpdated, nil
		}
	} else if !kerrors.IsNotFound(err) {
		// A real error occurred trying to get the Route. Fail fast.
		r.logger.Error(err, "Failed to get existing Route for pre-check")
		return nil, controllerutil.OperationResultNone, err
	}

	// err was not found so carry-on with creating a new route
	targetRoute := oresources.NewDefaultRoute(hawtio)

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, targetRoute, func() error {
		// A read-only copy of the cluster state for diff logging
		liveSnapshot := targetRoute.DeepCopy()
		// The current route's host field
		existingHost := liveSnapshot.Spec.Host

		// Set the owner reference for garbage collection.
		if err := controllerutil.SetControllerReference(hawtio, targetRoute, r.scheme); err != nil {
			return err
		}

		reqLogger := hawtioLogger.WithName(fmt.Sprintf("%s-reconcileRoute", hawtio.Name))
		blueprint := oresources.NewRoute(hawtio, deploymentConfig.tlsRouteSecret, deploymentConfig.caCertRouteSecret, reqLogger)

		serverBlueprint, err := hydrateDefaults(ctx, r.client, blueprint, func(source, hydrated *routev1.Route) {
			// If hydration stripped required fields, patch them directly back from source
			if hydrated.Spec.To.Kind == "" || hydrated.Spec.To.Name == "" {
				hydrated.Spec.To = source.Spec.To
			}
			if hydrated.Spec.Port == nil && source.Spec.Port != nil {
				hydrated.Spec.Port = source.Spec.Port
			}
			if hydrated.Spec.TLS == nil && source.Spec.TLS != nil {
				hydrated.Spec.TLS = source.Spec.TLS
			}

			// The hydration generates a host field based on the temporary name
			// (e.g. "hawtio-dry-run-123..."). This must be updated, using the
			// live cluster's existingHost or discarded.
			if source.Spec.Host == "" {
				if existingHost != "" {
					// Restore the live cluster Host
					hydrated.Spec.Host = existingHost
				} else {
					// Discard the garbage temporary host
					hydrated.Spec.Host = ""
				}
			}
		})
		if err != nil {
			return err
		}

		targetRoute.Labels = util.MergeMap(targetRoute.Labels, blueprint.Labels)
		targetRoute.Annotations = util.MergeMap(targetRoute.Annotations, serverBlueprint.Annotations)
		// Assign the fully hydrated and patched blueprint spec
		targetRoute.Spec = serverBlueprint.Spec

		// Report any known differences to the log (only if in debug log level)
		util.ReportDiff("Route", liveSnapshot, targetRoute)

		return nil
	})

	// There is a resource but the default client cache cannot see it
	// since it is legacy and not labelled. The create failed so it
	// should be adopted and reconciled.
	if err != nil && kerrors.IsAlreadyExists(err) {
		adoptionErr := r.adoptLegacyResource(ctx, oresources.NewDefaultRoute(hawtio))
		if adoptionErr != nil {
			// If adoption failed (e.g., API error) or an adoption was called
			// return any of these errors
			return nil, controllerutil.OperationResultNone, adoptionErr
		}
	}

	if err != nil {
		return nil, opResult, err
	}

	util.ReportResourceChange("Route", targetRoute, opResult)
	return targetRoute, opResult, nil
}

func (r *ReconcileHawtio) reconcileIngress(ctx context.Context, hawtio *hawtiov2.Hawtio, deploymentConfig DeploymentConfiguration) (*networkingv1.Ingress, controllerutil.OperationResult, error) {
	// Only create an ingress if confirmed as not a route version of Openshift
	if r.apiSpec.IsOpenShift4 && r.apiSpec.Routes {
		return nil, controllerutil.OperationResultNone, nil
	}

	targetIngress := kresources.NewDefaultIngress(hawtio)

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, targetIngress, func() error {
		// A read-only copy of the cluster state for diff logging
		liveSnapshot := targetIngress.DeepCopy()

		// Set the owner reference for garbage collection.
		if err := controllerutil.SetControllerReference(hawtio, targetIngress, r.scheme); err != nil {
			return err
		}

		reqLogger := hawtioLogger.WithName(fmt.Sprintf("%s-reconcileIngress", hawtio.Name))
		blueprint := kresources.NewIngress(hawtio, r.apiSpec, deploymentConfig.servingCertSecret, reqLogger)

		serverBlueprint, err := hydrateDefaults(ctx, r.client, blueprint, func(source, hydrated *networkingv1.Ingress) {
			// If hydration stripped required fields, patch them directly back from source
			if len(hydrated.Spec.Rules) == 0 {
				hydrated.Spec.Rules = source.Spec.Rules
			}

			if len(hydrated.Spec.TLS) == 0 && len(source.Spec.TLS) > 0 {
				hydrated.Spec.TLS = source.Spec.TLS
			}
		})
		if err != nil {
			return err
		}

		targetIngress.Labels = util.MergeMap(targetIngress.Labels, blueprint.Labels)
		targetIngress.Annotations = util.MergeMap(targetIngress.Annotations, serverBlueprint.Annotations)
		// Assign the fully hydrated and patched blueprint spec
		targetIngress.Spec = serverBlueprint.Spec

		// Report any known differences to the log (only if in debug log level)
		util.ReportDiff("Ingress", liveSnapshot, targetIngress)

		return nil
	})

	// There is a resource but the default client cache cannot see it
	// since it is legacy and not labelled. The create failed so it
	// should be adopted and reconciled.
	if err != nil && kerrors.IsAlreadyExists(err) {
		adoptionErr := r.adoptLegacyResource(ctx, kresources.NewDefaultIngress(hawtio))
		if adoptionErr != nil {
			// If adoption failed (e.g., API error) or an adoption was called
			// return any of these errors
			return nil, controllerutil.OperationResultNone, adoptionErr
		}
	}

	if err != nil {
		return nil, opResult, err
	}

	util.ReportResourceChange("Ingress", targetIngress, opResult)
	return targetIngress, opResult, nil
}
