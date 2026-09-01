package common

import (
	"errors"
	"time"

	errs "github.com/pkg/errors"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ErrLegacyResourceAdopted is a Sentinel error for
// denoting when a legacy resource has been adopted
var ErrLegacyResourceAdopted = errs.New("A legacy resource has been adopted, requeue required")

// RequeueError is a custom error type used to signal that the controller
// should gracefully requeue the reconciliation after a specific duration,
// without treating it as a crash or failure.
type RequeueError struct {
	Message      string
	RequeueAfter time.Duration
}

func (e *RequeueError) Error() string {
	return e.Message
}

// handleResultAndError
// If error is the Sentinel legacy adopted resource error
// then signal for a requeue. Otherwise, just return the error
func HandleResultAndError(err error) (reconcile.Result, error) {
	if err == nil {
		return reconcile.Result{}, nil
	}

	// Check for custom delayed requeue error
	var reqErr *RequeueError
	if errors.As(err, &reqErr) {
		// Return the delay, but return a NIL error so Kubernetes
		// respects the timer instead of triggering exponential backoff
		return reconcile.Result{RequeueAfter: reqErr.RequeueAfter}, nil
	}

	// Check for your existing legacy adoption
	if err == ErrLegacyResourceAdopted {
		return reconcile.Result{Requeue: true}, nil
	}

	// Fallback for actual system failures
	return reconcile.Result{}, err
}
