package hawtio

import (
	"context"
	"fmt"
	"time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"github.com/go-logr/logr"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/resources"
	"github.com/hawtio/hawtio-operator/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
)

func (r *ReconcileHawtio) reconcileDeployment(ctx context.Context, hawtio *hawtiov2.Hawtio, deploymentConfig DeploymentConfiguration) (controllerutil.OperationResult, error) {
	// The object the K8s client fetches into, and ultimately saves.
	targetDeployment := resources.NewDefaultDeployment(hawtio)
	reqLogger := hawtioLogger.WithName(fmt.Sprintf("%s-reconcileDeployment", hawtio.Name))

	onlineDigest, gatewayDigest := "", ""
	if r.updatePoller != nil {
		var digestErr error
		onlineDigest, gatewayDigest, digestErr = r.updatePoller.RequestDigests()

		if digestErr != nil {
			reqLogger.Info("Update Poller: Registry poller encountered an error; falling back to default image tags.", "reason", digestErr.Error())
		} else if onlineDigest == "" || gatewayDigest == "" {
			// If the update poller is active but its memory is still empty,
			// it means the initial network request hasn't finished yet.
			reqLogger.Info("Update Poller: Waiting for registry poller to fetch baseline digests...")
			// Abort this run, don't build anything, and try again in 2 seconds.
			// If the poller finishes sooner, its channel event will wake up the
			// reconcile again anyway
			// Return None, and our custom struct with the 2-second delay
			return controllerutil.OperationResultNone, &RequeueError{
				Message:      "waiting for initial registry baseline",
				RequeueAfter: 2 * time.Second,
			}
		}
	}

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, targetDeployment, func() error {
		// A read-only copy of the cluster state for diff logging
		liveSnapshot := targetDeployment.DeepCopy()

		// Set the owner reference for garbage collection
		if err := controllerutil.SetControllerReference(hawtio, targetDeployment, r.scheme); err != nil {
			return err
		}

		clientCertSecretVersion := ""
		if deploymentConfig.clientCertSecret != nil {
			reqLogger.V(util.DebugLogLevel).Info("Assigning to deployment client certificate secret", "Resource Version", deploymentConfig.clientCertSecret.GetResourceVersion())
			clientCertSecretVersion = deploymentConfig.clientCertSecret.GetResourceVersion()
		}

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

		r.addImageDigests(hawtio, targetDeployment, onlineDigest, gatewayDigest, reqLogger)

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

func (r *ReconcileHawtio) addImageDigests(hawtio *hawtiov2.Hawtio, deployment *appsv1.Deployment, onlineDigest string, gatewayDigest string, logger logr.Logger) {
	logger.V(util.DebugLogLevel).Info("Adding Update Poller digests to deployment", "onlineDigest", onlineDigest, "gatewayDigest", gatewayDigest)

	if r.updatePoller == nil {
		logger.V(util.DebugLogLevel).Info("Update Poller disabled. No modifications to deployment")
		return // nothing to do - no update poller initialized
	}

	if onlineDigest == "" || gatewayDigest == "" {
		logger.V(util.DebugLogLevel).Info("Update Poller digests are empty. No modifications to deployment")
		return // digests never populated so don't overwrite anything
	}

	logger.V(util.DebugLogLevel).Info("Adding Update Poller annotations to deployment")
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}

	logger.V(util.DebugLogLevel).Info("Modifying deployment images with Update Poller digests")
	// Loop through the containers in the Deployment
	for i, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == hawtio.Name+"-container" && onlineDigest != "" {
			// Track it in metadata
			deployment.Spec.Template.Annotations[resources.OnlineDigestAnnotation] = onlineDigest

			// Swap the image to use the immutable digest instead of the tag
			// eg. changes "quay.io/hawtio/online:2.4.0" -> "quay.io/hawtio/online@sha256:..."
			deployment.Spec.Template.Spec.Containers[i].Image = r.ImageRepository + "@" + onlineDigest
		}

		if container.Name == hawtio.Name+"-gateway-container" && gatewayDigest != "" {
			deployment.Spec.Template.Annotations[resources.GatewayDigestAnnotation] = gatewayDigest
			deployment.Spec.Template.Spec.Containers[i].Image = r.GatewayImageRepository + "@" + gatewayDigest
		}
	}
}
