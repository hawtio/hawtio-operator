package common

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	"github.com/hawtio/hawtio-operator/pkg/resources"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

// ReconcileBase is a base reconciler for the hawtio implementations
type ReconcileBase struct {
	util.BuildVariables
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the API server
	Client client.Client
	Scheme *runtime.Scheme
	// Reads directly from the live cluster so maybe ahead of the client
	// cache. Only used sparingly for secrets.
	ApiReader client.Reader
	// Derivative go-client version of apiReader
	CoreClient       corev1client.CoreV1Interface
	ApiSpec          *capabilities.ApiServerSpec
	Logger           logr.Logger
	OperatorPod      types.NamespacedName
	CertExpiryPeriod time.Duration
}

// LogOperationResult logs completion of a resource operation
func (r *ReconcileBase) LogOperationResult(resource string, result controllerutil.OperationResult) {
	if result == controllerutil.OperationResultNone {
		return // no need to log occasions where no action was taken
	}

	r.Logger.Info("=== Resource "+resource+" Reconciliation Completed ===", "Result", result)
}

// AdoptLegacyResource upgrades resources from older versions / manual installs
// Such resources may not have a label so cannot be 'seen' by the client's cache.
// So, need to adopt the resource by providing it with a label and requeuing.
func (r *ReconcileBase) AdoptLegacyResource(ctx context.Context, obj client.Object) error {
	r.Logger.Info(fmt.Sprintf("Adopting legacy resource [Type: %T] [Name: %s/%s]", obj, obj.GetNamespace(), obj.GetName()))
	key := client.ObjectKeyFromObject(obj)

	// Use API Reader to bypass cache and obtain legacy resource
	if err := r.ApiReader.Get(ctx, key, obj); err != nil {
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

	r.Logger.V(util.DebugLogLevel).Info(fmt.Sprintf("Self-healing: Adopting legacy resource %s/%s by adding missing label", obj.GetNamespace(), obj.GetName()))

	labels[resources.LabelAppKey] = resources.LabelAppValue
	obj.SetLabels(labels)

	// Update the object (triggers a new Reconcile event)
	if err := r.Client.Update(ctx, obj); err != nil {
		// Update failed so return that error
		return err
	}

	// Return adopted error to signal that reconcile should be
	// requeued immediately and object should be found in the cache
	return ErrLegacyResourceAdopted
}

// calculateSecretHash generates a deterministic SHA-256 hash of the secret's data map payload.
func (r *ReconcileBase) CalculateSecretHash(secret *corev1.Secret) string {
	if secret == nil || len(secret.Data) == 0 {
		return "empty"
	}

	// Extract and sort the map keys to guarantee a fixed order
	keys := make([]string, 0, len(secret.Data))
	for k := range secret.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Initialize the crypto writer
	hash := sha256.New()

	// Write data into the hash engine in the exact same sequence every time
	for _, k := range keys {
		// Write the key name to guard against key swapping changes
		hash.Write([]byte(k))
		// Write the raw bytes
		hash.Write(secret.Data[k])
	}

	// Compute the checksum and return a short 6-8 character suffix
	fullHash := hex.EncodeToString(hash.Sum(nil))
	return fullHash[:8]
}
