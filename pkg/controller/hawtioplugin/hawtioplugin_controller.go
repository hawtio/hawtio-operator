package hawtioplugin

import (
	"context"
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hawtiopluginv1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtioplugin/v1"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	"github.com/hawtio/hawtio-operator/pkg/clients"
	commonctrl "github.com/hawtio/hawtio-operator/pkg/controller/common"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

const (
	HawtioUnderTestEnvVar = "HAWTIO_UNDER_TEST"
)

var pluginLogger = logf.Log.WithName("controller_hawtioplugin")

// ReconcileHawtioPlugin reconciles a HawtioPlugin object
type ReconcileHawtioPlugin struct {
	commonctrl.ReconcileBase
}

// Add creates a new Hawtio Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, operatorPod types.NamespacedName, clientTools *clients.ClientTools, apiSpec *capabilities.ApiServerSpec, bv util.BuildVariables, certExpiryPeriod time.Duration) error {
	r := &ReconcileHawtioPlugin{
		ReconcileBase: commonctrl.ReconcileBase{
			BuildVariables:   bv,
			Client:           mgr.GetClient(),
			Scheme:           mgr.GetScheme(),
			ApiReader:        mgr.GetAPIReader(),
			CoreClient:       clientTools.CoreClient,
			ApiSpec:          apiSpec,
			OperatorPod:      operatorPod,
			CertExpiryPeriod: certExpiryPeriod,
		},
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
	r.Logger = pluginLogger.WithValues("Operator Namespace", r.OperatorPod.Namespace, "Request.Name", request.Name)
	r.Logger.Info("Reconciling cluster-level HawtioPlugin")

	r.Logger.V(util.DebugLogLevel).Info(fmt.Sprintf("Cluster API Specification: %+v", r.ApiSpec))

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}
