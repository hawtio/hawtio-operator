package hawtio

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"

	"github.com/hawtio/hawtio-operator/pkg/util"
)

func (r *ReconcileHawtio) setHawtioPhase(ctx context.Context, hawtio *hawtiov2.Hawtio, phase hawtiov2.HawtioPhase) error {
	r.logger.V(util.DebugLogLevel).Info("Setting Hawtio CR Phase:", "Phase", phase)

	if hawtio.Status.Phase != phase {
		previous := hawtio.DeepCopy()
		hawtio.Status.Phase = phase
		err := r.client.Status().Patch(ctx, hawtio, client.MergeFrom(previous))
		if err != nil {
			return fmt.Errorf("failed to update hawtio phase to %s: %v", phase, err)
		}
	}

	return nil
}

// isDeploymentFailed checks if the Deployment has exceeded its progress deadline.
func (r *ReconcileHawtio) isDeploymentFailed(deployment *appsv1.Deployment) bool {
	for _, cond := range deployment.Status.Conditions {
		// Check for the specific Type and Reason that indicate failure
		if cond.Type == appsv1.DeploymentProgressing && cond.Reason == "ProgressDeadlineExceeded" {
			return true
		}
	}
	return false
}
