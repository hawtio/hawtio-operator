package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	errs "github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	oauthv1 "github.com/openshift/api/oauth/v1"
	routev1 "github.com/openshift/api/route/v1"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/hawtio/hawtio-operator/pkg/apis"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	"github.com/hawtio/hawtio-operator/pkg/clients"
	"github.com/hawtio/hawtio-operator/pkg/controller/hawtio"
	"github.com/hawtio/hawtio-operator/pkg/updater"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

var log = logf.Log.WithName("manager")

// ConfigureScheme register all the kubernetes resource types required
// in a custom scheme
func ConfigureScheme() (*runtime.Scheme, error) {
	var scheme = runtime.NewScheme()

	err := clientgoscheme.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = oauthv1.Install(scheme)
	if err != nil {
		return nil, err
	}
	err = routev1.Install(scheme)
	if err != nil {
		return nil, err
	}
	err = configv1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = consolev1.Install(scheme)
	if err != nil {
		return nil, err
	}
	err = apiextensionsv1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	// Register Hawtio api scheme
	err = apis.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	return scheme, nil
}

// createCacheOptions
// Restrict resource watching to only those resources with the app/hawtio label
func createCacheOptions(watchNamespaces string, apiSpec *capabilities.ApiServerSpec) cache.Options {
	lblReq, _ := labels.NewRequirement("app", selection.Equals, []string{"hawtio"})
	selector := labels.NewSelector().Add(*lblReq)

	// Configure namespace scope
	var namespaces map[string]cache.Config
	if watchNamespaces != "" {
		namespaces = make(map[string]cache.Config)
		// Split the string by comma
		nsList := strings.Split(watchNamespaces, ",")
		// Loop through the list, trim whitespace, and add each to the map
		for _, ns := range nsList {
			cleanNs := strings.TrimSpace(ns)
			if cleanNs != "" {
				namespaces[cleanNs] = cache.Config{}
			}
		}
	}

	cacheOptions := cache.Options{
		DefaultNamespaces: namespaces,
		ByObject: map[client.Object]cache.ByObject{
			&appsv1.Deployment{}:    {Label: selector},
			&corev1.ConfigMap{}:     {Label: selector},
			&corev1.Secret{}:        {Label: selector},
			&networkingv1.Ingress{}: {Label: selector},
		},
	}

	// Conditional Route Use
	// Check if the cluster actually supports Routes
	// before adding them to the Cache Watch.
	if apiSpec.Routes {
		log.Info("OpenShift Route API detected. Enabling Route support.")
		cacheOptions.ByObject[&routev1.Route{}] = cache.ByObject{Label: selector}
	}

	return cacheOptions
}

// mgrConfig options for constructing the manager
// Use With... functions to populate
type mgrConfig struct {
	// Required
	restConfig      *rest.Config
	watchNamespaces string
	operatorPodNS   string
	buildVariables  util.BuildVariables
	// Optional
	scheme                *runtime.Scheme
	clientTools           *clients.ClientTools
	metrics               metricserver.Options
	updatePollingInterval time.Duration
	registryTransport     http.RoundTripper
}

// MgrOption function to populate manager config
type MgrOption func(*mgrConfig)

// PollerConfig provides config parameters for creation
// of the RegistryPoller (see createUpdatePoller)
type PollerConfig struct {
	Manager         manager.Manager
	Namespace       string
	BuildVars       util.BuildVariables
	PollingInterval time.Duration
	ExtraOptions    []remote.Option
}

// customPullSecretNameEnvVar is the constant for env variable CUSTOM_PULL_SECRET_NAME
// can specify the name of a custom pull secret in the operator's namespace
// An empty value means the operator will either try and find a global pull secret
// (only if on OpenShift) or poll the image registry with no authentication.
const customPullSecretNameEnvVar = "CUSTOM_PULL_SECRET_NAME"

// WithRestConfig allows an external rest config to be defined
func WithRestConfig(cfg *rest.Config) MgrOption {
	return func(c *mgrConfig) {
		c.restConfig = cfg
	}
}

// WithWatchNamespaces allows an external watch namespace to be defined
func WithWatchNamespaces(nsStr string) MgrOption {
	return func(c *mgrConfig) {
		c.watchNamespaces = nsStr
	}
}

// WithPodNamespace allows an external pod namespace to be defined
func WithPodNamespace(ns string) MgrOption {
	return func(c *mgrConfig) {
		c.operatorPodNS = ns
	}
}

// WithUpdatePollingInterval defines polling interval for updater of disables it
func WithUpdatePollingInterval(interval time.Duration) MgrOption {
	return func(c *mgrConfig) {
		c.updatePollingInterval = interval
	}
}

// WithBuildVariables allows an external build variables to be defined
func WithBuildVariables(bv util.BuildVariables) MgrOption {
	return func(c *mgrConfig) {
		c.buildVariables = bv
	}
}

// WithScheme allows an external scheme to be defined
func WithScheme(scheme *runtime.Scheme) MgrOption {
	return func(c *mgrConfig) {
		c.scheme = scheme
	}
}

// WithClientTools allows an external client tools to be defined
func WithClientTools(tools *clients.ClientTools) MgrOption {
	return func(c *mgrConfig) {
		c.clientTools = tools
	}
}

// WithMetrics allows an external metrics to be defined
func WithMetrics(metrics metricserver.Options) MgrOption {
	return func(c *mgrConfig) {
		c.metrics = metrics
	}
}

// WithRegistryTransport allows an external transport for accessing a registry
func WithRegistryTransport(transport http.RoundTripper) MgrOption {
	return func(c *mgrConfig) {
		c.registryTransport = transport
	}
}

// New creates a controller-runtime Manager
// - Uses the custom scheme
// - Configures 'app=hawtio' Cache Filtering (Memory Optimization)
func New(mgrOptions ...MgrOption) (manager.Manager, error) {

	//
	// Evaluate the manager options and assemble the config
	//
	mc := &mgrConfig{}
	for _, apply := range mgrOptions {
		// applies each option function to the new manager config
		apply(mc)
	}

	// Fail fast if the critical config is missing
	if mc.restConfig == nil {
		return nil, fmt.Errorf("rest.Config must be provided in Options")
	}

	// Initialise the scheme if not provided
	if mc.scheme == nil {
		scheme, err := ConfigureScheme()
		if err != nil {
			return nil, err
		}
		mc.scheme = scheme
	}

	// mc.watchNamespaces can be empty as it will act in cluster mode

	if len(mc.operatorPodNS) == 0 {
		return nil, fmt.Errorf("The operator pod namespace must be specified")
	}

	// mc.buildVariables can be empty

	if mc.clientTools == nil {
		clientTools, err := clients.NewTools(mc.restConfig)
		if err != nil {
			return nil, err
		}

		mc.clientTools = clientTools
	}

	// mc.metrics can be empty

	ctx := context.Background()

	// Identify cluster capabilities
	apiSpec, err := capabilities.APICapabilities(ctx, mc.clientTools.ApiClient, mc.clientTools.ConfigClient)
	if err != nil {
		return nil, errs.Wrap(err, "Cluster API capability discovery failed")
	}

	//
	// Initialise the manager
	//

	cacheOptions := createCacheOptions(mc.watchNamespaces, apiSpec)

	podName, found := os.LookupEnv("POD_NAME")
	if !found {
		return nil, fmt.Errorf("POD_NAME environment variable is not set")
	}
	operatorPod := types.NamespacedName{
		Name:      podName,
		Namespace: mc.operatorPodNS,
	}

	// construct the manager
	mgr, err := manager.New(mc.restConfig, manager.Options{
		Scheme:                  mc.scheme,
		Cache:                   cacheOptions,
		LeaderElectionNamespace: mc.operatorPodNS,
		Metrics:                 mc.metrics,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to construct manager: %w", err)
	}

	var extraOptions []remote.Option
	if mc.registryTransport != nil {
		extraOptions = append(extraOptions, remote.WithTransport(mc.registryTransport))
	}

	//
	// Polling interval will be set by default but if the user
	// has explicitly disabled then don't create the poller or channel
	//
	cfg := PollerConfig{
		Manager:         mgr,
		Namespace:       operatorPod.Namespace,
		BuildVars:       mc.buildVariables,
		PollingInterval: mc.updatePollingInterval,
		ExtraOptions:    extraOptions,
	}

	updatePoller, updateChannel, err := createUpdatePoller(ctx, cfg)
	if err != nil {
		// Force the poller and channel to nil to ensure they are disabled.
		log.Error(err, "Unable to construct update poller. Auto-updates will be disabled.")
		updatePoller = nil
		updateChannel = nil
	}

	// Register the hawtio controller with the manager
	if err := hawtio.Add(
		mgr, operatorPod, mc.clientTools,
		apiSpec, mc.buildVariables,
		updatePoller, updateChannel); err != nil {
		return nil, err
	}

	return mgr, nil
}

func createUpdatePoller(ctx context.Context, cfg PollerConfig) (*updater.RegistryPoller, chan event.GenericEvent, error) {
	if cfg.PollingInterval == 0 {
		log.Info("Update Poller: Image polling is disabled (interval is 0). Background updater will not be started.")
		return nil, nil, nil
	}

	registryCreds, err := discoverRegistryCredentials(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}

	pollerKeychain, err := parseKeychain(registryCreds)
	if err != nil {
		return nil, nil, err
	}

	//
	// Creates a bi-directional channel but with downgrade
	// to receive-only when assigned to ReconcileHawtio
	//
	updateChannel := make(chan event.GenericEvent)

	poller := &updater.RegistryPoller{
		Interval:        cfg.PollingInterval,
		OnlineImageURL:  cfg.BuildVars.ImageRepository + ":" + cfg.BuildVars.ImageVersion,
		GatewayImageURL: cfg.BuildVars.GatewayImageRepository + ":" + cfg.BuildVars.GatewayImageVersion,
		Trigger:         updateChannel,
		AuthKeychain:    pollerKeychain,
		Logger:          log.WithName("Update Poller"),
		ExtraOptions:    cfg.ExtraOptions,
	}

	if err := cfg.Manager.Add(poller); err != nil {
		log.Error(err, "Update Poller: failed to add registry poller to manager")
		return nil, nil, err
	}

	return poller, updateChannel, nil
}

func discoverRegistryCredentials(ctx context.Context, cfg PollerConfig) ([]byte, error) {
	secret := &corev1.Secret{}

	// Try the custom secret in the Operator's namespace
	customSecretName := os.Getenv(customPullSecretNameEnvVar)
	if customSecretName != "" {
		err := cfg.Manager.GetAPIReader().Get(ctx, client.ObjectKey{Namespace: cfg.Namespace, Name: customSecretName}, secret)
		if err != nil {
			// Fail on all errors as user specified CUSTOM_PULL_SECRET_NAME
			return nil, fmt.Errorf("CUSTOM_PULL_SECRET_NAME was specified but the secret could not be retained: %w", err)
		}

		log.V(util.DebugLogLevel).Info("Secret obtained from CUSTOM_PULL_SECRET_NAME")

		dockerConfigJSON, exists := secret.Data[corev1.DockerConfigJsonKey]
		if !exists {
			return nil, fmt.Errorf("(CUSTOM_PULL_SECRET_NAME) Secret %s exists but does not contain a %s key; is it a valid docker-registry secret?", customSecretName, corev1.DockerConfigJsonKey)
		}

		return dockerConfigJSON, nil
	}

	// Nothing found, proceed anonymously
	return nil, nil
}

func parseKeychain(configBytes []byte) (authn.Keychain, error) {
	// If no secret was found, return an empty keychain that always resolves to Anonymous
	if len(configBytes) == 0 {
		return &updater.DockerConfigKeychain{Auths: make(map[string]authn.AuthConfig)}, nil
	}

	var config struct {
		Auths map[string]authn.AuthConfig `json:"auths"`
	}

	if err := json.Unmarshal(configBytes, &config); err != nil {
		// If JSON parsing fails, return the error
		return nil, err
	}

	return &updater.DockerConfigKeychain{Auths: config.Auths}, nil
}
