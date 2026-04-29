package updater

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// ErrRegistryUnavailable is returned when the remote registry
// cannot be reached due to network constraints (air-gapped)
// or rate-limiting timeouts.
var ErrRegistryUnavailable = errors.New("registry connection failed or timed out")

// GetLatestDigest fetches the latest digest of the image
// url from the container registry.
// TODO: Handle authentication using a pullSecrets array
func GetLatestDigest(ctx context.Context, imageURL string, extraOpts ...remote.Option) (string, error) {
	// Parse the image string into a structured reference
	ref, err := name.ParseReference(imageURL)
	if err != nil {
		return "", err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Setup remote options, injecting timeout context
	options := []remote.Option{
		remote.WithContext(timeoutCtx),
	}

	// Append any injected extra options
	// Used in testing
	options = append(options, extraOpts...)

	// Perform a HEAD request
	descriptor, err := remote.Head(ref, options...)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrRegistryUnavailable, err)
	}

	// Return the sha256 digest string
	return descriptor.Digest.String(), nil
}
