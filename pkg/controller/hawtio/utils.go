package hawtio

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ReconcileHawtio) logOperationResult(resource string, result controllerutil.OperationResult) {
	if result == controllerutil.OperationResultNone {
		return // no need to log occasions where no action was taken
	}

	r.logger.Info("=== Resource "+resource+" Reconciliation Completed ===", "Result", result)
}

func (r *ReconcileHawtio) getOperatorPod(ctx context.Context, namespacedName client.ObjectKey) (*corev1.Pod, error) {
	pod := &corev1.Pod{}
	err := r.client.Get(ctx, namespacedName, pod)

	if err != nil {
		hawtioLogger.Error(err, "Pod not found")
		return nil, err
	}
	return pod, nil
}
