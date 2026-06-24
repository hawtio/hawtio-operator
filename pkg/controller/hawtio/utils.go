package hawtio

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ReconcileHawtio) logOperationResult(resource string, result controllerutil.OperationResult) {
	if result == controllerutil.OperationResultNone {
		return // no need to log occasions where no action was taken
	}

	r.logger.Info("=== Resource "+resource+" Reconciliation Completed ===", "Result", result)
}

// calculateSecretHash generates a deterministic SHA-256 hash of the secret's data map payload.
func (r *ReconcileHawtio) calculateSecretHash(secret *corev1.Secret) string {
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
