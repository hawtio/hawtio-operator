package hawtio

import (
	"context"
	"errors"
	"fmt"
	"time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	consolev1 "github.com/openshift/api/console/v1"
	oauthv1 "github.com/openshift/api/oauth/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/resources"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

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
func handleResultAndError(err error) (reconcile.Result, error) {
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

func (r *ReconcileHawtio) fetchHawtio(ctx context.Context, namespacedName client.ObjectKey) (*hawtiov2.Hawtio, error) {
	r.logger.V(util.DebugLogLevel).Info("Fetching the Hawtio custom resource")

	hawtio := hawtiov2.NewHawtio()
	err := r.client.Get(ctx, namespacedName, hawtio)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			r.logger.V(util.DebugLogLevel).Info("No Hawtio CR found")
			return nil, nil
		}
		// Error reading the object - requeue the request.
		return nil, err
	}

	return hawtio, nil
}

func (r *ReconcileHawtio) handleDeletion(ctx context.Context, hawtio *hawtiov2.Hawtio, namespacedName client.ObjectKey) (bool, error) {
	if hawtio.GetDeletionTimestamp() == nil {
		return false, nil
	}

	r.logger.V(util.DebugLogLevel).Info("=== Deleting Installation ===")
	err := r.deletion(ctx, hawtio, namespacedName)
	if err != nil {
		return true, fmt.Errorf("deletion failed: %v", err)
	}
	// Deletion was successful (or is in progress),
	// return true to tell Reconcile() to stop.
	return true, nil
}

func (r *ReconcileHawtio) deletion(ctx context.Context, hawtio *hawtiov2.Hawtio, namespacedName client.ObjectKey) error {
	if controllerutil.ContainsFinalizer(hawtio, "foregroundDeletion") {
		return nil
	}

	clientName := namespacedName.Name + "-" + namespacedName.Namespace

	if r.apiSpec.IsOpenShift4 {
		if hawtio.Spec.Type == hawtiov2.ClusterHawtioDeploymentType {
			// Remove URI from OAuth client
			oc := &oauthv1.OAuthClient{}
			clientName := namespacedName.Name + "-" + namespacedName.Namespace
			err := r.client.Get(ctx, types.NamespacedName{Name: clientName}, oc)
			if err != nil && !kerrors.IsNotFound(err) {
				return fmt.Errorf("failed to get OAuth client: %v", err)
			}
			updated := resources.RemoveRedirectURIFromOauthClient(oc, hawtio.Status.URL)
			if updated {
				err := r.client.Update(ctx, oc)
				if err != nil {
					return fmt.Errorf("failed to remove redirect URI from OAuth client: %v", err)
				}
			}
		}

		// Remove OpenShift console link
		consoleLink := &consolev1.ConsoleLink{
			ObjectMeta: metav1.ObjectMeta{
				Name: clientName,
			},
		}
		err := r.client.Delete(ctx, consoleLink)
		if err != nil && !kerrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
			return fmt.Errorf("failed to delete console link: %v", err)
		}
	}

	// Check if the finalizer is still present before trying to remove it
	if controllerutil.ContainsFinalizer(hawtio, hawtioFinalizer) {
		previous := hawtio.DeepCopy()
		controllerutil.RemoveFinalizer(hawtio, hawtioFinalizer)

		// Use patch rather than update to ensure only the
		// explicit changes are merged in rather than potentially
		// overwriting with a stale hawtio CR
		err := r.client.Patch(ctx, hawtio, client.MergeFrom(previous))
		if err != nil {
			return fmt.Errorf("failed to remove finalizer: %v", err)
		}
	}

	return nil
}

func (r *ReconcileHawtio) addFinalizer(ctx context.Context, hawtio *hawtiov2.Hawtio) (bool, error) {
	// Add a finalizer, that's needed to clean up cluster-wide resources, like ConsoleLink and OAuthClient
	if controllerutil.ContainsFinalizer(hawtio, hawtioFinalizer) {
		r.logger.V(util.DebugLogLevel).Info("Finalizer already present")
		return false, nil // nothing to do
	}

	r.logger.V(util.DebugLogLevel).Info("Adding a finalizer")

	previous := hawtio.DeepCopy()
	controllerutil.AddFinalizer(hawtio, hawtioFinalizer)

	// Use patch rather than update to ensure only the
	// explicit changes are merged in rather than potentially
	// overwriting with a stale hawtio CR
	err := r.client.Patch(ctx, hawtio, client.MergeFrom(previous))
	if err != nil {
		r.logger.V(util.DebugLogLevel).Info("Error finalizer")
		return false, fmt.Errorf("failed to update finalizer: %v", err)
	}

	r.logger.V(util.DebugLogLevel).Info("Completed finalizer")
	return true, nil
}
