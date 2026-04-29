package hawtio

import (
	"context"
	"fmt"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/hawtio/hawtio-operator/pkg/resources"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

// adoptLegacyResource
// If upgrading from older version or overwriting a manual install it's possible a
// resource may not have a label so cannot be 'seen' by the client's cache. So, need
// to adopt the resource by providing it with a label and requeuing.
func (r *ReconcileHawtio) adoptLegacyResource(ctx context.Context, obj client.Object) error {
	r.logger.Info(fmt.Sprintf("Adopting legacy resource [Type: %T] [Name: %s/%s]", obj, obj.GetNamespace(), obj.GetName()))
	key := client.ObjectKeyFromObject(obj)

	// Use API Reader to bypass cache and obtain legacy resource
	if err := r.apiReader.Get(ctx, key, obj); err != nil {
		if kerrors.IsNotFound(err) {
			// It really doesn't exist. The 'AlreadyExists' error was a race condition or ghost.
			return nil
		}
		// Some other error that needs to be logged
		return err
	}

	// Check/Add the Missing Label
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	// Safety: If label exists, we shouldn't be here. Avoid infinite loops.
	if val, ok := labels[resources.LabelAppKey]; ok && val == resources.LabelAppValue {
		return nil
	}

	r.logger.V(util.DebugLogLevel).Info(fmt.Sprintf("Self-healing: Adopting legacy resource %s/%s by adding missing label", obj.GetNamespace(), obj.GetName()))

	labels[resources.LabelAppKey] = resources.LabelAppValue
	obj.SetLabels(labels)

	// Update the object (triggers a new Reconcile event)
	if err := r.client.Update(ctx, obj); err != nil {
		// Update failed so return that error
		return err
	}

	// Return adopted error to signal that reconcile should be
	// requeued immediately and object should be found in the cache
	return ErrLegacyResourceAdopted
}
