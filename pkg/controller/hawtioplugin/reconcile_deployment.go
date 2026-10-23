package hawtioplugin

import (
	"context"
	"fmt"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	hawtiopluginv1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtioplugin/v1"
	"github.com/hawtio/hawtio-operator/pkg/cfg"
	commonctrl "github.com/hawtio/hawtio-operator/pkg/controller/common"
	gatewayres "github.com/hawtio/hawtio-operator/pkg/resources/hawtioplugin/gateway"
	"github.com/hawtio/hawtio-operator/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
)

func (r *ReconcileHawtioPlugin) reconcileGatewayDeployment(ctx context.Context, hawtioPlugin *hawtiopluginv1.HawtioPlugin, deploymentConfig cfg.DeploymentConfiguration) (controllerutil.OperationResult, error) {
	namespace := r.OperatorPod.Namespace

	// The object the K8s client fetches into, and ultimately saves.
	targetDeployment := gatewayres.NewDefaultDeployment(namespace)
	reqLogger := r.Logger.WithName(fmt.Sprintf("%s-reconcileDeployment", targetDeployment.Name))

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, targetDeployment, func() error {
		// A read-only copy of the cluster state for diff logging
		liveSnapshot := targetDeployment.DeepCopy()

		// Set the owner reference for garbage collection
		if err := controllerutil.SetControllerReference(hawtioPlugin, targetDeployment, r.Scheme); err != nil {
			return err
		}

		// Local, ideal state generated from the Hawtio CR
		blueprint, err := gatewayres.NewDeployment(hawtioPlugin, namespace,
			r.BuildVariables, deploymentConfig, reqLogger)
		if err != nil {
			reqLogger.Error(err, "Error reconciling deployment")
			return err
		}

		serverBlueprint, err := commonctrl.HydrateDefaults(ctx, r.Client, blueprint, func(source, hydrated *appsv1.Deployment) {
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
		adoptionErr := r.AdoptLegacyResource(ctx, gatewayres.NewDefaultDeployment(namespace))
		if adoptionErr != nil {
			// If adoption failed (e.g., API error) or an adoption was called
			// return any of these errors
			return controllerutil.OperationResultNone, adoptionErr
		}
	}

	util.ReportResourceChange("Deployment", targetDeployment, opResult)
	return opResult, err
}
