package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/hawtio/hawtio-operator/pkg/util"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/events"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// hawtioPullSecretName is the constant for the name of the pull
// secret in the operator's namespace.
// If no secret with this name exists thens the operator will
// try to poll the image registry with no authentication.
const hawtioPullSecretName = "hawtio-pull-secret"

// RegistryPoller checks the remote registry on a schedule
// and fires an event if the image changes.
type RegistryPoller struct {
	Interval        time.Duration
	OperatorRef     *corev1.ObjectReference
	OnlineImageURL  string
	GatewayImageURL string
	APIReader       client.Reader
	Logger          logr.Logger
	EventEmitter    events.EventRecorder
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
		msg := "Update Poller: Image polling disabled (interval is 0)"
		p.Logger.Info(msg)
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
			msg := "Update Poller: Stopping registry poller"
			p.Logger.Info(msg)
			p.EventEmitter.Eventf(p.OperatorRef, nil, corev1.EventTypeNormal, "ImageUpdateStopped", "Polling", msg)
			return nil
		case <-ticker.C:
			p.checkRegistry(ctx)
		}
	}
}

func (p *RegistryPoller) publishError(err error, msg string) {
	p.Logger.Error(err, msg)
	p.EventEmitter.Eventf(p.OperatorRef, nil, corev1.EventTypeWarning, "ImageUpdateFailed", "Polling", "%s Error: %v", msg, err)

	p.mu.Lock()
	p.lastError = err
	p.mu.Unlock()
}

func (p *RegistryPoller) checkRegistry(ctx context.Context) {
	p.Logger.V(util.DebugLogLevel).Info("Update Poller: Polling registry for new digests", "online image", p.OnlineImageURL, "gateway image", p.GatewayImageURL)

	registryCreds, err := p.discoverRegistryCredentials(ctx)
	if err != nil {
		msg := "Update Poller: Failure discoving pull secret. Skipping cycle."
		p.publishError(err, msg)
		return // Fail open: if one fails, we skip the whole cycle to keep them synced
	}

	authKeychain, err := p.parseKeychain(registryCreds)
	if err != nil {
		msg := "Update Poller: Failed to parse pull secret. Skipping cycle."
		p.publishError(err, msg)
		return // Fail open: if one fails, we skip the whole cycle to keep them synced
	}

	// Check Online Image
	newOnlineDigest, errOnline := GetLatestDigest(ctx, p.OnlineImageURL, authKeychain, p.ExtraOptions...)
	p.Logger.V(util.DebugLogLevel).Info("Update Poller: New Online Digest:", "digest", newOnlineDigest)

	if errOnline != nil {
		msg := "Update Poller: Failed to check Online image registry. Skipping cycle."
		p.publishError(errOnline, msg)
		return // Fail open: if one fails, we skip the whole cycle to keep them synced
	}

	// Check Gateway Image
	newGatewayDigest, errGateway := GetLatestDigest(ctx, p.GatewayImageURL, authKeychain, p.ExtraOptions...)
	p.Logger.V(util.DebugLogLevel).Info("Update Poller: New Online Gateway Digest:", "digest", newGatewayDigest)
	if errGateway != nil {
		msg := "Update Poller: Failed to check Gateway image registry. Skipping cycle."
		p.publishError(errGateway, msg)
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
		msg := "Update Poller: New Hawtio images found! Triggering cluster-wide rollout"
		p.Logger.Info(msg,
			"onlineUpdated", onlineChanged,
			"gatewayUpdated", gatewayChanged)
		p.EventEmitter.Eventf(p.OperatorRef, nil, corev1.EventTypeNormal, "ImageUpdateChange", "Polling", "%s - onlineUpdated: %t - gatewayUpdated: %t", msg, onlineChanged, gatewayChanged)

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

func (p *RegistryPoller) discoverRegistryCredentials(ctx context.Context) ([]byte, error) {
	secret := &corev1.Secret{}

	// Try the pull secret in the Operator's namespace
	err := p.APIReader.Get(ctx, client.ObjectKey{Namespace: p.OperatorRef.Namespace, Name: hawtioPullSecretName}, secret)
	if err != nil && kerrors.IsNotFound(err) {
		// No secret present, proceed anonymously
		return nil, nil
	} else if err != nil {
		// Fail on all errors as user specified CUSTOM_PULL_SECRET_NAME
		return nil, fmt.Errorf("Error occurred obtaining %s secret: %w", hawtioPullSecretName, err)
	}

	p.Logger.V(util.DebugLogLevel).Info("Secret obtained", "name", hawtioPullSecretName)

	dockerConfigJSON, exists := secret.Data[corev1.DockerConfigJsonKey]
	if !exists {
		return nil, fmt.Errorf("Secret '%s' exists but does not contain a %s key; is it a valid docker-registry secret?", hawtioPullSecretName, corev1.DockerConfigJsonKey)
	}

	return dockerConfigJSON, nil
}

func (p *RegistryPoller) parseKeychain(configBytes []byte) (authn.Keychain, error) {
	// If no secret was found, return an empty keychain that always resolves to Anonymous
	if len(configBytes) == 0 {
		return &DockerConfigKeychain{Auths: make(map[string]authn.AuthConfig)}, nil
	}

	var config struct {
		Auths map[string]authn.AuthConfig `json:"auths"`
	}

	if err := json.Unmarshal(configBytes, &config); err != nil {
		// If JSON parsing fails, return the error
		return nil, err
	}

	return &DockerConfigKeychain{Auths: config.Auths}, nil
}
