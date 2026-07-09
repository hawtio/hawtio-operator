package manager

import (
	"context"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	OperatorPod     types.NamespacedName
	BuildVars       util.BuildVariables
	PollingInterval time.Duration
	ExtraOptions    []remote.Option
}

//
// Create a function that will run after the Manager has started
//
type deferredTask func(context.Context) error

func (f deferredTask) Start(ctx context.Context) error {
	return f(ctx)
}

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
		OperatorPod:     operatorPod,
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
	operatorRef, err := getOperatorRef(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}

	eventEmitter := cfg.Manager.GetEventRecorder("hawtio-update-poller")

	if cfg.PollingInterval == 0 {
		msg := "Update Poller: Image polling is disabled (interval is 0). Background updater will not be started."
		log.Info(msg, "operator-ref", operatorRef)

		// Wrap the emitter in a deferredTask so it broadcasts the event
		// after the Manager has properly started
		err := cfg.Manager.Add(deferredTask(func(ctx context.Context) error {
			eventEmitter.Eventf(operatorRef, nil, corev1.EventTypeNormal, "ImageUpdateDisabled", "Init", msg)
			return nil
		}))
		if err != nil {
			return nil, nil, err
		}

		return nil, nil, nil
	}

	//
	// Creates a bi-directional channel but with downgrade
	// to receive-only when assigned to ReconcileHawtio
	//
	updateChannel := make(chan event.GenericEvent)

	poller := &updater.RegistryPoller{
		Interval:        cfg.PollingInterval,
		OperatorRef:     operatorRef,
		OnlineImageURL:  cfg.BuildVars.ImageRepository + ":" + cfg.BuildVars.ImageVersion,
		GatewayImageURL: cfg.BuildVars.GatewayImageRepository + ":" + cfg.BuildVars.GatewayImageVersion,
		Trigger:         updateChannel,
		APIReader:       cfg.Manager.GetAPIReader(),
		Logger:          log.WithName("Hawtio Update Poller"),
		EventEmitter:    eventEmitter,
		ExtraOptions:    cfg.ExtraOptions,
	}

	if err := cfg.Manager.Add(poller); err != nil {
		log.Error(err, "Update Poller: failed to add registry poller to manager")
		return nil, nil, err
	}

	return poller, updateChannel, nil
}

func getOperatorRef(ctx context.Context, cfg PollerConfig) (*corev1.ObjectReference, error) {
	apiReader := cfg.Manager.GetAPIReader()
	operatorPod := cfg.OperatorPod

	pod := &corev1.Pod{}
	err := apiReader.Get(ctx, operatorPod, pod)
	if err != nil {
		return nil, err
	}

	// Get the ReplicaSet Name from the Pod's OwnerReferences
	podOwner := metav1.GetControllerOf(pod)
	if podOwner == nil || podOwner.Kind != "ReplicaSet" {
		// Fallback to owning Pod if not managed by a ReplicaSet (e.g., local testing)
		return &corev1.ObjectReference{
			Kind:       "Pod",
			APIVersion: "core/v1",
			Name:       pod.GetName(),
			Namespace:  pod.GetNamespace(),
			UID:        pod.GetUID(),
		}, nil
	}

	// Fetch the ReplicaSet
	replicaSet := &appsv1.ReplicaSet{}
	err = apiReader.Get(ctx, client.ObjectKey{Namespace: operatorPod.Namespace, Name: podOwner.Name}, replicaSet)
	if err != nil {
		return nil, err
	}

	// Get the Deployment Name from the ReplicaSet's OwnerReferences
	rsOwner := metav1.GetControllerOf(replicaSet)
	if rsOwner == nil || rsOwner.Kind != "Deployment" {
		// Fallback to ReplicaSet if no Deployment owner exists
		return &corev1.ObjectReference{
			Kind:       "ReplicaSet",
			APIVersion: "apps/v1",
			Name:       replicaSet.GetName(),
			Namespace:  replicaSet.GetNamespace(),
			UID:        replicaSet.GetUID(),
		}, nil
	}

	// Fetch the Deployment struct
	deployment := &appsv1.Deployment{}
	err = apiReader.Get(ctx, client.ObjectKey{Namespace: operatorPod.Namespace, Name: rsOwner.Name}, deployment)
	if err != nil {
		return nil, err
	}

	return &corev1.ObjectReference{
		Kind:       "Deployment",
		APIVersion: "apps/v1",
		Name:       deployment.GetName(),
		Namespace:  deployment.GetNamespace(),
		UID:        deployment.GetUID(),
	}, nil
}
