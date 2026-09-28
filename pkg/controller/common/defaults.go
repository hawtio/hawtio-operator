package common

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)


// HydrateDefaults performs a server-side Dry-Run Create to populate the
// 'blueprint' object with all the default values (Spec, Status, etc.) that
// the specific cluster applies.
// It accepts an optional ensureSpecIntegrity closure to validate that
// critical fields remain present and if not restore them from the blueprint
// object.
// It returns the "hydrated" object.
func HydrateDefaults[T client.Object](ctx context.Context, c client.Client, blueprint T, ensureSpecIntegrity func(source T, hydrated T)) (T, error) {
	// DeepCopy to avoid mutating the input object in case the caller reuses it
	// Cast the result back to T (which works because T is a pointer to a struct)
	obj := blueprint.DeepCopyObject().(T)

	// OpenShift OAuthClient validation rejects it.
	obj.SetGenerateName("")

	// Generate a unique, valid name.
	// Ends with a number, satisfying the [a-z0-9] regex constraint.
	// Use a very short name (t + 7 digits) to prevent DNS length errors in Routes.
	// Example: t4829102
	obj.SetName(fmt.Sprintf("t%d", time.Now().UnixNano()%10000000))

	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetSelfLink("")
	obj.SetCreationTimestamp(metav1.Time{})
	// Clear OwnerReferences just in case, though DryRun usually ignores them
	obj.SetOwnerReferences(nil)

	// Perform Dry-Run Create
	// The API server will populate defaults and return the object
	if err := c.Create(ctx, obj, client.DryRunAll); err != nil {
		// We return the 'zero' value of T and the error
		var zero T
		return zero, fmt.Errorf("failed to hydrate defaults via dry-run: %w", err)
	}

	// Validate with the ensureSpecIntegrity function if one was provided
	// to patch any dropped fields
	if ensureSpecIntegrity != nil {
		ensureSpecIntegrity(blueprint, obj)
	}

	return obj, nil
}
