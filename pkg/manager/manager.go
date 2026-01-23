package manager

import (
	"context"
	"fmt"

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
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/hawtio/hawtio-operator/pkg/apis"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	"github.com/hawtio/hawtio-operator/pkg/clients"
	"github.com/hawtio/hawtio-operator/pkg/controller/hawtio"
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
func createCacheOptions(watchNamespace string, apiSpec *capabilities.ApiServerSpec) cache.Options {
	lblReq, _ := labels.NewRequirement("app", selection.Equals, []string{"hawtio"})
	selector := labels.NewSelector().Add(*lblReq)

	// Configure namespace scope
	var namespaces map[string]cache.Config
	if watchNamespace != "" {
		namespaces = map[string]cache.Config{watchNamespace: {}}
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
	restConfig     *rest.Config
	watchNamespace string
	podNamespace   string
	buildVariables util.BuildVariables
	// Optional
	scheme      *runtime.Scheme
	clientTools *clients.ClientTools
	metrics     metricserver.Options
}

// MgrOption function to populate manager config
type MgrOption func(*mgrConfig)

// WithRestConfig allows an external rest config to be defined
func WithRestConfig(cfg *rest.Config) MgrOption {
	return func(c *mgrConfig) {
		c.restConfig = cfg
	}
}

// WithWatchNamespace allows an external watch namespace to be defined
func WithWatchNamespace(ns string) MgrOption {
	return func(c *mgrConfig) {
		c.watchNamespace = ns
	}
}

// WithPodNamespace allows an external pod namespace to be defined
func WithPodNamespace(ns string) MgrOption {
	return func(c *mgrConfig) {
		c.podNamespace = ns
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

	// mc.watchNamespace can be empty as it will act in cluster mode

	if len(mc.podNamespace) == 0 {
		return nil, fmt.Errorf("The pod namespace must be specified")
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

	// Identify cluster capabilities
	apiSpec, err := capabilities.APICapabilities(context.TODO(), mc.clientTools.ApiClient, mc.clientTools.ConfigClient)
	if err != nil {
		return nil, errs.Wrap(err, "Cluster API capability discovery failed")
	}

	//
	// Initialise the manager
	//

	cacheOptions := createCacheOptions(mc.watchNamespace, apiSpec)

	// construct the manager
	mgr, err := manager.New(mc.restConfig, manager.Options{
		Scheme:                  mc.scheme,
		Cache:                   cacheOptions,
		LeaderElectionNamespace: mc.podNamespace,
		Metrics:                 mc.metrics,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to construct manager: %w", err)
	}

	// Register the hawtio controller with the manager
	if err := hawtio.Add(mgr, mc.clientTools, apiSpec, mc.buildVariables); err != nil {
		return nil, err
	}

	return mgr, nil
}
