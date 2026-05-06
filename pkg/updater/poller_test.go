package updater

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// MockRegistryTransport intercepts HTTP calls and returns fake image digests.
type MockRegistryTransport struct {
	mu sync.Mutex
	// Maps a registry URL path to the sha256 digest we want to return
	DigestMap  map[string][]string
	ShouldFail bool
}

func (m *MockRegistryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.ShouldFail {
		return nil, http.ErrServerClosed // Simulate a hard network/air-gap failure
	}

	// Intercept the mandatory Docker API version ping
	if req.URL.Path == "/v2/" {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString("{}")),
			Header:     make(http.Header),
		}, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	digests, ok := m.DigestMap[req.URL.Path]
	if !ok || len(digests) == 0 {
		errMsg := "mock missing path: " + req.URL.Path
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString(errMsg)),
			Header:     make(http.Header),
		}, nil
	}

	// Get the current digest for this request
	digest := digests[0]

	// If there's another digest in the sequence, pop it so the next
	// request gets the new one
	if len(digests) > 1 {
		m.DigestMap[req.URL.Path] = digests[1:]
	}

	// go-containerregistry strictly requires this header to extract the digest
	header := make(http.Header)
	header.Set("Docker-Content-Digest", digest)
	header.Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("{}")),
		Header:     header,
	}, nil
}

// This proves that if the registry returns a new set of digests on the second tick,
// the Poller triggers the channel with an event and makes the new digests available.
func TestRegistryPoller_ExpectedUpdate(t *testing.T) {
	onlineHash := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	updatedOnlineHash := "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	gatewayHash := "sha256:3333333333333333333333333333333333333333333333333333333333333333"
	updatedGatewayHash := "sha256:4444444444444444444444444444444444444444444444444444444444444444"

	// Setup mock transport with initial digests
	mockTransport := &MockRegistryTransport{
		DigestMap: map[string][]string{
			"/v2/hawtio/online/manifests/latest":         {onlineHash, updatedOnlineHash},
			"/v2/hawtio/online-gateway/manifests/latest": {gatewayHash, updatedGatewayHash},
		},
	}

	triggerChan := make(chan event.GenericEvent, 1) // Buffer of 1 so it doesn't block

	poller := &RegistryPoller{
		Interval:        10 * time.Millisecond, // Run almost instantly
		OnlineImageURL:  "quay.io/hawtio/online:latest",
		GatewayImageURL: "quay.io/hawtio/online-gateway:latest",
		Logger: testr.NewWithOptions(t, testr.Options{
			Verbosity: 1,
		}),
		Trigger: triggerChan,
		extraOptions: []remote.Option{
			remote.WithTransport(mockTransport), // Inject the mock!
		},
	}

	// Start the poller in the background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = poller.Start(ctx)
	}()

	// Wait for the initial baseline event
	select {
	case <-triggerChan:
		t.Log("Baseline event fired successfully")
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out waiting for baseline event")
	}

	// Wait for the 10ms ticker to fire the simulated registry update
	select {
	case <-triggerChan:
		t.Log("Update event fired successfully")
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out waiting for update event")
	}

	// Validate the internal memory state
	online, gateway, err := poller.RequestDigests()
	assert.NoError(t, err)
	assert.Equal(t, updatedOnlineHash, online)
	assert.Equal(t, updatedGatewayHash, gateway)
}

// This proves that if the registry returns the exact same digests on the second tick,
// the Poller gracefully ignores them and doesn't spam the channel.
func TestRegistryPoller_Idempotency(t *testing.T) {
	validHash1 := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	validHash2 := "sha256:2222222222222222222222222222222222222222222222222222222222222222"

	mockTransport := &MockRegistryTransport{
		DigestMap: map[string][]string{
			// Notice the sequence has two of the EXACT SAME hashes
			"/v2/hawtio/online/manifests/latest":         {validHash1, validHash1},
			"/v2/hawtio/online-gateway/manifests/latest": {validHash2, validHash2},
		},
	}

	triggerChan := make(chan event.GenericEvent, 1)
	poller := &RegistryPoller{
		Interval:        10 * time.Millisecond,
		OnlineImageURL:  "quay.io/hawtio/online:latest",
		GatewayImageURL: "quay.io/hawtio/online-gateway:latest",
		Trigger:         triggerChan,
		Logger:          testr.New(t),
		extraOptions:    []remote.Option{remote.WithTransport(mockTransport)},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = poller.Start(ctx) }()

	// The baseline check should fire immediately
	select {
	case <-triggerChan:
		t.Log("Baseline event fired successfully")
	case <-time.After(1 * time.Second):
		t.Fatal("Test timed out waiting for baseline event")
	}

	baseOnline, baseGateway, err := poller.RequestDigests()
	assert.NoError(t, err)

	// The second tick happens 10ms later, but because the hashes are identical,
	// it should NOT fire an event into the channel.
	select {
	case <-triggerChan:
		t.Fatal("Poller fired a duplicate event when the digests did not change!")
	case <-time.After(100 * time.Millisecond):
		t.Log("Success! Poller properly ignored identical digests.")
	}

	online, gateway, err := poller.RequestDigests()
	assert.NoError(t, err)
	assert.Equal(t, baseOnline, online)
	assert.Equal(t, baseGateway, gateway)
}

// This proves that if the network goes down, the Poller doesn't crash the
// operator. It just logs the error and waits for the next cycle.
func TestRegistryPoller_AirGap(t *testing.T) {
	// Force the mock to simulate a hard network failure
	mockTransport := &MockRegistryTransport{
		ShouldFail: true,
	}

	triggerChan := make(chan event.GenericEvent, 1)
	poller := &RegistryPoller{
		Interval:        10 * time.Millisecond,
		OnlineImageURL:  "quay.io/hawtio/online:latest",
		GatewayImageURL: "quay.io/hawtio/online-gateway:latest",
		Trigger:         triggerChan,
		Logger:          testr.New(t),
		extraOptions:    []remote.Option{remote.WithTransport(mockTransport)},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = poller.Start(ctx) }()

	// Because the network failed, the baseline event should NEVER fire
	select {
	case <-triggerChan:
		t.Fatal("Poller fired an event despite a complete network failure!")
	case <-time.After(100 * time.Millisecond):
		t.Log("Success! Poller gracefully swallowed the network error.")
	}

	// Ensure memory remained empty
	online, gateway, err := poller.RequestDigests()
	assert.Error(t, err)
	assert.Equal(t, "", online)
	assert.Equal(t, "", gateway)
}

// This proves that if one image is found but the other isn't,
// the Poller rejects the cycle to prevent tearing the deployment
// (updating one container but not the other).
func TestRegistryPoller_PartialFailure(t *testing.T) {
	validHash := "sha256:1111111111111111111111111111111111111111111111111111111111111111"

	mockTransport := &MockRegistryTransport{
		DigestMap: map[string][]string{
			// Online succeeds...
			"/v2/hawtio/online/manifests/latest": {validHash},
			// ...but Gateway is missing from the map, simulating a 404
		},
	}

	triggerChan := make(chan event.GenericEvent, 1)
	poller := &RegistryPoller{
		Interval:        10 * time.Millisecond,
		OnlineImageURL:  "quay.io/hawtio/online:latest",
		GatewayImageURL: "quay.io/hawtio/online-gateway:latest",
		Trigger:         triggerChan,
		Logger:          testr.New(t),
		extraOptions:    []remote.Option{remote.WithTransport(mockTransport)},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = poller.Start(ctx) }()

	// Because the gateway failed, the whole cycle should be aborted
	select {
	case <-triggerChan:
		t.Fatal("Poller fired an event despite a partial fetch failure!")
	case <-time.After(100 * time.Millisecond):
		t.Log("Success! Poller aborted the cycle to keep images synchronized.")
	}

	online, gateway, err := poller.RequestDigests()
	assert.Error(t, err)
	assert.Equal(t, "", online)
	assert.Equal(t, "", gateway)
}
