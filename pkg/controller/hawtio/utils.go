package hawtio

import (
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ReconcileHawtio) logOperationResult(resource string, result controllerutil.OperationResult) {
	if result == controllerutil.OperationResultNone {
		return // no need to log occasions where no action was taken
	}

	r.logger.Info("=== Resource "+resource+" Reconciliation Completed ===", "Result", result)
}
