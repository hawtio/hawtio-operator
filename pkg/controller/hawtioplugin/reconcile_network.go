package hawtioplugin

import (
	"context"
	"fmt"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	corev1 "k8s.io/api/core/v1"

	hawtiopluginv1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtioplugin/v1"
	"github.com/hawtio/hawtio-operator/pkg/cfg"
	commonctrl "github.com/hawtio/hawtio-operator/pkg/controller/common"
	gatewayres "github.com/hawtio/hawtio-operator/pkg/resources/hawtioplugin/gateway"
	"github.com/hawtio/hawtio-operator/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ReconcileHawtioPlugin) reconcileGatewayService(ctx context.Context, hawtioPlugin *hawtiopluginv1.HawtioPlugin, namespace string, config cfg.DeploymentConfiguration) (controllerutil.OperationResult, error) {
	targetService := gatewayres.NewDefaultService(namespace)

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, targetService, func() error {
		// A read-only copy of the cluster state for diff logging
		liveSnapshot := targetService.DeepCopy()
		oldClusterIP := targetService.Spec.ClusterIP
		oldClusterIPs := targetService.Spec.ClusterIPs

		// Set the owner reference for garbage collection.
		if err := controllerutil.SetControllerReference(hawtioPlugin, targetService, r.Scheme); err != nil {
			return err
		}

		reqLogger := r.Logger.WithName(fmt.Sprintf("%s-reconcileService", hawtioPlugin.Name))
		blueprint := gatewayres.NewService(hawtioPlugin, namespace, config, reqLogger)

		serverBlueprint, err := commonctrl.HydrateDefaults(ctx, r.Client, blueprint, func(source, hydrated *corev1.Service) {
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
		adoptionErr := r.AdoptLegacyResource(ctx, gatewayres.NewDefaultService(namespace))
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
