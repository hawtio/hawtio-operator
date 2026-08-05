package hawtioplugin

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kclient "k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configclient "github.com/openshift/client-go/config/clientset/versioned"
	oauthclient "github.com/openshift/client-go/oauth/clientset/versioned"

	hawtiopluginv1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtioplugin/v1"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	"github.com/hawtio/hawtio-operator/pkg/clients"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

const (
	HawtioUnderTestEnvVar = "HAWTIO_UNDER_TEST"
)

var pluginLogger = logf.Log.WithName("controller_hawtioplugin")

// ReconcileHawtioPlugin reconciles a HawtioPlugin object
type ReconcileHawtioPlugin struct {
	util.BuildVariables
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the API server
	client           client.Client
	scheme           *runtime.Scheme
	apiReader        client.Reader
	coreClient       corev1client.CoreV1Interface
	oauthClient      oauthclient.Interface
	configClient     configclient.Interface
	apiClient        kclient.Interface
	apiSpec          *capabilities.ApiServerSpec
	logger           logr.Logger
	operatorPod      types.NamespacedName
	certExpiryPeriod time.Duration
}

// Add creates a new Hawtio Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, operatorPod types.NamespacedName, clientTools *clients.ClientTools, apiSpec *capabilities.ApiServerSpec, bv util.BuildVariables, certExpiryPeriod time.Duration) error {
	r := &ReconcileHawtioPlugin{
		BuildVariables:   bv,
		client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		apiReader:        mgr.GetAPIReader(),
		coreClient:       clientTools.CoreClient,
		oauthClient:      clientTools.OAuthClient,
		configClient:     clientTools.ConfigClient,
		apiClient:        clientTools.ApiClient,
		apiSpec:          apiSpec,
		operatorPod:      operatorPod,
		certExpiryPeriod: certExpiryPeriod,
	}

	if err := r.setupWithManager(mgr); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileHawtioPlugin{}

// setupWithManager sets up the controller with the Manager.
func (r *ReconcileHawtioPlugin) setupWithManager(mgr ctrl.Manager) error {
	// Need to skip name registry validation if
	// the controller is being run through test suites
	skipValidation := false
	if os.Getenv(HawtioUnderTestEnvVar) == "true" {
		skipValidation = true
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&hawtiopluginv1.HawtioPlugin{}).
		WithOptions(controller.Options{
			SkipNameValidation: &skipValidation,
		}).
		Named("hawtioplugin-controller").
		// Watches(/* ... */).
		// Owns(&appsv1.Deployment{}).
		Complete(r)
}

// Reconcile is responsible for processing the HawtioPlugin CR and other watched resources
func (r *ReconcileHawtioPlugin) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	r.logger = pluginLogger.WithValues("Operator Namespace", r.operatorPod.Namespace, "Request.Name", request.Name)
	r.logger.Info("Reconciling cluster-level HawtioPlugin")

	r.logger.V(util.DebugLogLevel).Info(fmt.Sprintf("Cluster API Specification: %+v", r.apiSpec))

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}
