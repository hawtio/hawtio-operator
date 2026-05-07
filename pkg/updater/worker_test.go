//go:build integration

package updater

import (
	"context"
	"strings"
	"testing"
	"time"
)

const fetchAttempts = 3

func retryGetDigest(imageURL string) (string, error) {
	var digest string
	var err error

	// Retry up to 3 times
	for i := 0; i < fetchAttempts; i++ {
		// Use a generous timeout for CI
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

		digest, err = GetLatestDigest(ctx, imageURL)
		cancel()

		if err == nil {
			return digest, nil // Success!
		}

		time.Sleep(2 * time.Second) // Small backoff before retrying
	}

	return digest, err
}

func TestGetLatestDigest_PublicQuayImage(t *testing.T) {
	// Public hawtio image of quay
	imageURL := "quay.io/hawtio/online:2.4.0"

	// Call our function with nil secrets
	digest, err := retryGetDigest(imageURL)
	if err != nil {
		t.Fatalf("Failed to fetch digest after %d attempts: %v", fetchAttempts, err)
	}

	// Basic validation
	if digest == "" {
		t.Errorf("Expected a valid digest, got an empty string")
	}

	if !strings.HasPrefix(digest, "sha256:") {
		t.Errorf("Expected digest to start with 'sha256:', got: %s", digest)
	}

	t.Logf("Success! Retrieved digest for %s: %s", imageURL, digest)
}

func TestGetLatestDigest_AirGappedSimulation(t *testing.T) {
	// .invalid is a reserved TLD guaranteed to fail DNS resolution instantly.
	// Simulates a disconnected/air-gapped cluster trying to reach the outside world.
	imageURL := "quay.invalid/hawtio/online:latest"

	digest, err := retryGetDigest(imageURL)

	if err == nil {
		t.Fatalf("Expected a network error for an unreachable registry, but got nil with digest: %s", digest)
	}

	// Verify the error is actually a network/resolution error, not a parsing error
	if !strings.Contains(err.Error(), "no such host") && !strings.Contains(err.Error(), "dial tcp") {
		t.Errorf("Expected a network-related 'no such host' error, got: %v", err)
	}

	t.Logf("Success! Caught air-gap network failure gracefully: %v", err)
}

func TestGetLatestDigest_InvalidImageReference(t *testing.T) {
	// A structurally invalid image string to test parsing
	// OCI conventions state that container urls cannot contain capitals
	imageURL := "quay.io/HAWTIO/online:latest"

	_, err := retryGetDigest(imageURL)

	if err == nil {
		t.Fatal("Expected an error for an invalid image string, but got nil")
	}

	if !strings.Contains(err.Error(), "could not parse reference") {
		t.Errorf("Expected a parsing error, got: %v", err)
		t.Fail()
	} else {
		t.Logf("Success! Caught invalid reference gracefully: %v", err)
	}
}
