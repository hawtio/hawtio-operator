package updater

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/hawtio/hawtio-operator/pkg/util"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// RegistryPoller checks the remote registry on a schedule
// and fires an event if the image changes.
type RegistryPoller struct {
	Interval        time.Duration
	OnlineImageURL  string
	GatewayImageURL string
	AuthKeychain    authn.Keychain
	Logger          logr.Logger
	Trigger         chan event.GenericEvent // bi-directional channel
	mu              sync.RWMutex
	onlineDigest    string
	gatewayDigest   string
	lastError       error

	// ExtraOptions used to inject any extra options into polling
	// Used for testing in mocking the HTTP transport.
	ExtraOptions []remote.Option
}

// RequestDigests is called by the Reconciler to
// read the cached digest without making network calls.
func (p *RegistryPoller) RequestDigests() (string, string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.onlineDigest, p.gatewayDigest, p.lastError
}

// Start fulfills the manager.Runnable interface.
func (p *RegistryPoller) Start(ctx context.Context) error {
	p.Logger.V(util.DebugLogLevel).Info("Update Poller: Updater polling check")

	if p.Interval == 0 {
		p.Logger.Info("Update Poller: Image polling disabled (interval is 0)")
		<-ctx.Done()
		return nil
	}

	// Fetch the baseline so Reconcilers have it from the start of the operator
	p.Logger.Info("Update Poller: Conducting baseline registry check", "online image", p.OnlineImageURL, "gateway image", p.GatewayImageURL)
	p.checkRegistry(ctx)

	p.Logger.Info("Update Poller: Starting registry poller", "interval", p.Interval.String(), "online image", p.OnlineImageURL, "gateway image", p.GatewayImageURL)
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.Logger.Info("Update Poller: Stopping registry poller")
			return nil
		case <-ticker.C:
			p.checkRegistry(ctx)
		}
	}
}

func (p *RegistryPoller) checkRegistry(ctx context.Context) {
	p.Logger.V(util.DebugLogLevel).Info("Update Poller: Polling registry for new digests", "online image", p.OnlineImageURL, "gateway image", p.GatewayImageURL)

	// Check Online Image
	newOnlineDigest, errOnline := GetLatestDigest(ctx, p.OnlineImageURL, p.AuthKeychain, p.ExtraOptions...)
	p.Logger.V(util.DebugLogLevel).Info("Update Poller: New Online Digest:", "digest", newOnlineDigest)

	if errOnline != nil {
		p.Logger.Error(errOnline, "Update Poller: Failed to check Online image registry. Skipping cycle.")
		p.mu.Lock()
		p.lastError = errOnline
		p.mu.Unlock()
		return // Fail open: if one fails, we skip the whole cycle to keep them synced
	}

	// Check Gateway Image
	newGatewayDigest, errGateway := GetLatestDigest(ctx, p.GatewayImageURL, p.AuthKeychain, p.ExtraOptions...)
	p.Logger.V(util.DebugLogLevel).Info("Update Poller: New Online Gateway Digest:", "digest", newGatewayDigest)
	if errGateway != nil {
		p.Logger.Error(errGateway, "Update Poller: Failed to check Gateway image registry. Skipping cycle.")
		p.mu.Lock()
		p.lastError = errGateway
		p.mu.Unlock()
		return
	}

	p.mu.Lock()
	// Clear the error on successful fetch
	p.lastError = nil
	// Check if digests have changed
	onlineChanged := p.onlineDigest != newOnlineDigest
	gatewayChanged := p.gatewayDigest != newGatewayDigest

	p.Logger.V(util.DebugLogLevel).Info("Update Poller: Checked changed images:", "onlineChanged", onlineChanged, "gatewayChanged", gatewayChanged)

	p.onlineDigest = newOnlineDigest
	p.gatewayDigest = newGatewayDigest
	p.mu.Unlock()

	// Only trigger if we had previous data, and at least one image updated
	if onlineChanged || gatewayChanged {
		p.Logger.Info("Update Poller: New Hawtio images found! Triggering cluster-wide rollout",
			"onlineUpdated", onlineChanged,
			"gatewayUpdated", gatewayChanged)

		p.Trigger <- event.GenericEvent{
			Object: &metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hawtio-global-update",
					Namespace: "",
				},
			},
		}
	}
}
