//go:build integration

package updater

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestGetLatestDigest_PublicQuayImage(t *testing.T) {
	// Public hawtio image of quay
	imageURL := "quay.io/hawtio/online:2.4.0"

	// Create a context with a timeout to simulate the operator environment
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Call our function with nil secrets
	digest, err := GetLatestDigest(ctx, imageURL)
	if err != nil {
		t.Fatalf("Failed to fetch digest: %v", err)
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

	ctx := context.Background()

	digest, err := GetLatestDigest(ctx, imageURL)

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

	ctx := context.Background()

	_, err := GetLatestDigest(ctx, imageURL)

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
