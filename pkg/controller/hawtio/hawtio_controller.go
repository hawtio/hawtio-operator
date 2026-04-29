package hawtio

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"time"

	errs "github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kclient "k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	routev1 "github.com/openshift/api/route/v1"
	configclient "github.com/openshift/client-go/config/clientset/versioned"
	oauthclient "github.com/openshift/client-go/oauth/clientset/versioned"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	"github.com/hawtio/hawtio-operator/pkg/openshift"
	kresources "github.com/hawtio/hawtio-operator/pkg/resources/kubernetes"
	oresources "github.com/hawtio/hawtio-operator/pkg/resources/openshift"
	"github.com/hawtio/hawtio-operator/pkg/util"
	"github.com/hawtio/hawtio-operator/pkg/clients"
)

var hawtioLogger = logf.Log.WithName("controller_hawtio")

const (
	hawtioFinalizer         = "hawt.io/finalizer"
	HawtioUnderTestEnvVar   = "HAWTIO_UNDER_TEST"
)

var ErrLegacyResourceAdopted = errs.New("A legacy resource has been adopted, requeue required")

func enqueueRequestForOwner[T client.Object](mgr manager.Manager) handler.TypedEventHandler[T, reconcile.Request] {
	return handler.TypedEnqueueRequestForOwner[T](mgr.GetScheme(), mgr.GetRESTMapper(), hawtiov2.NewHawtio(), handler.OnlyControllerOwner())
}

// Add creates a new Hawtio Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, operatorPod types.NamespacedName, clientTools *clients.ClientTools, apiSpec *capabilities.ApiServerSpec, bv util.BuildVariables) error {
	r := &ReconcileHawtio{
		BuildVariables: bv,
		client:         mgr.GetClient(),
		scheme:         mgr.GetScheme(),
		apiReader:      mgr.GetAPIReader(),
		coreClient:     clientTools.CoreClient,
		oauthClient:    clientTools.OAuthClient,
		configClient:   clientTools.ConfigClient,
		apiClient:      clientTools.ApiClient,
		apiSpec:        apiSpec,
		operatorPod:    operatorPod,
	}

	if r.apiSpec.IsOpenShift4 {
		if err := openshift.ConsoleYAMLSampleExists(); err == nil {
			openshift.CreateConsoleYAMLSamples(context.TODO(), mgr.GetClient(), r.ProductName)
		}
	}

	// Need to skip name registry validation if
	// the controller is being run through test suites
	skipValidation := false
	if os.Getenv(HawtioUnderTestEnvVar) == "true" {
		skipValidation = true
  }

	// Create a new controller
	c, err := controller.New("hawtio-controller", mgr, controller.Options{
		Reconciler: r,
		SkipNameValidation: &skipValidation,
	})
	if err != nil {
		return errs.Wrap(err, "Failed to create new controller")
	}

	// Watch for changes to primary resource Hawtio

	err = c.Watch(
		source.Kind(
			mgr.GetCache(), hawtiov2.NewHawtio(),
			&handler.TypedEnqueueRequestForObject[*hawtiov2.Hawtio]{},
			predicate.TypedFuncs[*hawtiov2.Hawtio]{
				UpdateFunc: func(e event.TypedUpdateEvent[*hawtiov2.Hawtio]) bool {
					// Ignore updates to CR status in which case metadata.Generation does not change
					return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*hawtiov2.Hawtio]) bool {
					// Evaluates to false if the object has been confirmed deleted
					return !e.DeleteStateUnknown
				},
			},
		),
	)
	if err != nil {
		return errs.Wrap(err, "Failed to create watch for Hawtio resource")
	}

	// Watch for changes to secondary resources and requeue the owner Hawtio
	err = c.Watch(source.Kind(mgr.GetCache(), &corev1.ConfigMap{}, enqueueRequestForOwner[*corev1.ConfigMap](mgr)))
	if err != nil {
		return errs.Wrap(err, "Failed to create watch for ConfigMap resource")
	}

	if r.apiSpec.Routes {
		err = c.Watch(source.Kind(mgr.GetCache(), &routev1.Route{}, enqueueRequestForOwner[*routev1.Route](mgr)))
		if err != nil {
			return errs.Wrap(err, "Failed to create watch for Route resource")
		}
	} else {
		err = c.Watch(source.Kind(mgr.GetCache(), &networkingv1.Ingress{}, enqueueRequestForOwner[*networkingv1.Ingress](mgr)))
		if err != nil {
			return errs.Wrap(err, "Failed to create watch for Ingress resource")
		}
	}

	err = c.Watch(
		source.Kind(
			mgr.GetCache(), &appsv1.Deployment{}, enqueueRequestForOwner[*appsv1.Deployment](mgr),
			predicate.TypedFuncs[*appsv1.Deployment]{
				UpdateFunc: func(e event.TypedUpdateEvent[*appsv1.Deployment]) bool {
					oldDeployment := e.ObjectOld
					newDeployment := e.ObjectNew
					// Filter events to reduce noise, but ensuring we wake up for:
					// 1. Scaling events (Status.Replicas changes).
					// 2. Startup completion (Status.ReadyReplicas changes). This is critical
					//    to transition the CR Status from 'Initialized' to 'Deployed'.
					// 3. Configuration changes (Generation changes).
					return oldDeployment.Status.Replicas != newDeployment.Status.Replicas ||
						oldDeployment.Status.ReadyReplicas != newDeployment.Status.ReadyReplicas ||
						oldDeployment.Generation != newDeployment.Generation
				},
			},
		),
	)
	if err != nil {
		return errs.Wrap(err, "Failed to create watch for Deployment resource")
	}

	//watch secret
	err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, enqueueRequestForOwner[*corev1.Secret](mgr)))
	if err != nil {
		return errs.Wrap(err, "Failed to create watch for Secret resource")
	}

	return nil
}

// handleResultAndError
// If error is the Sentinel legacy adopted resource error
// then signal for a requeue. Otherwise, just return the error
func handleResultAndError(err error) (reconcile.Result, error) {
	if err == ErrLegacyResourceAdopted {
		// Adoption occurred so requeue
		return reconcile.Result{Requeue: true}, err
	}

	return reconcile.Result{}, err
}

var _ reconcile.Reconciler = &ReconcileHawtio{}

// ReconcileHawtio reconciles a Hawtio object
type ReconcileHawtio struct {
	util.BuildVariables
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the API server
	client       client.Client
	scheme       *runtime.Scheme
	apiReader    client.Reader
	coreClient   corev1client.CoreV1Interface
	oauthClient  oauthclient.Interface
	configClient configclient.Interface
	apiClient    kclient.Interface
	apiSpec      *capabilities.ApiServerSpec
	logger       logr.Logger
	operatorPod   types.NamespacedName
}

// DeploymentConfiguration acquires properties used in deployment
type DeploymentConfiguration struct {
	openShiftConsoleURL string
	configMap           *corev1.ConfigMap
	clientCertSecret    *corev1.Secret // -proxying certificate secret
	tlsRouteSecret      *corev1.Secret // custom route certificate secret
	caCertRouteSecret   *corev1.Secret // custom CA certificate secret
	servingCertSecret   *corev1.Secret // -serving certificate secret
}

// Reconcile reads that state of the cluster for a Hawtio object and makes changes based on the state read
// and what is in the Hawtio.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileHawtio) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	r.logger = hawtioLogger.WithValues("Operator Namespace", r.operatorPod, "Hawtio CR Namespace", request.Namespace, "Request.Name", request.Name)
	r.logger.Info(fmt.Sprintf("Reconciling Hawtio in %s", request.Namespace))

	r.logger.V(util.DebugLogLevel).Info(fmt.Sprintf("Cluster API Specification: %+v", r.apiSpec))

	crNamespacedName := request.NamespacedName
	opNamespacedName := r.operatorPod

	// =====================================================================
	// PHASE 1: SETUP & FETCH
	// =====================================================================
	// Fetch the Hawtio instance from the cluster.
	r.logger.V(util.DebugLogLevel).Info("=== Fetching Hawtio Custom Resource ===")
	hawtio, err := r.fetchHawtio(ctx, crNamespacedName)
	if (err != nil) {
		return reconcile.Result{}, err
	} else if hawtio == nil {
		// Request object not found, could have been deleted after reconcile request.
		// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
		// Return and don't requeue
		return reconcile.Result{}, nil
	}

	// =====================================================================
	// PHASE 2: DELETION AND FINALIZERS
	// =====================================================================
	// If install marked for deletion then go ahead and delete
	delete, err := r.handleDeletion(ctx, hawtio, crNamespacedName)
	if err != nil {
		return reconcile.Result{}, err
	} else if delete {
		return reconcile.Result{}, nil
	}

	// Aid deletion by adding finalizer
	r.logger.V(util.DebugLogLevel).Info("=== Add Finalizer ===")
	updated, err := r.addFinalizer(ctx, hawtio)
	if err != nil {
		return reconcile.Result{}, err
	} else if updated {
		return reconcile.Result{Requeue: true}, err
	}

	// =====================================================================
	// PHASE 3: INITIALIZE STATUS
	// =====================================================================
	// If the status phase is empty, it's a new CR, so we initialize it.
	r.logger.V(util.DebugLogLevel).Info("=== Verify Hawtio Install Mode ===")
	updated, err = r.verifyHawtioSpecType(ctx, hawtio)
	if err != nil {
		return reconcile.Result{}, err
	} else if updated {
		return reconcile.Result{Requeue: true}, err
	}

	// Check the status of the RBAC ConfigMap.
	// If specified in the CR then it should be present.
	r.logger.V(util.DebugLogLevel).Info("=== Verifying RBAC ConfigMap ===")
	valid, err := r.verifyRBACConfigMap(ctx, hawtio, crNamespacedName)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// Let's poll for the RBAC ConfigMap to be created
			return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
		} else {
			return reconcile.Result{}, err
		}
	} else if !valid {
		// Lets poll until the RBAC ConfigMap is valid
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	}

	if len(hawtio.Status.Phase) == 0 || hawtio.Status.Phase == hawtiov2.HawtioPhaseFailed {
		r.logger.V(util.DebugLogLevel).Info("Hawtio.Status.Phase is zero or failed. Setting to initialized.")
		err := r.setHawtioPhase(ctx, hawtio, hawtiov2.HawtioPhaseInitialized)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{Requeue: true}, nil
	}

	// =====================================================================
	// PHASE 4: RECONCILE AND DEPLOY PHASE
	// =====================================================================

	// Reconcile the service account
	// - may include OAuth Client annotations, if applicable
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling Service Account ===")
	opResult, err := r.reconcileServiceAccount(ctx, hawtio)
	r.logOperationResult("ServiceAccount", opResult)
	if err != nil {
		return handleResultAndError(err)
	}

	r.logger.V(util.DebugLogLevel).Info("=== Reconciling Service Account Role and Binding ===")
	opResult, err = r.reconcileServiceAccountRole(ctx, hawtio)
	r.logOperationResult("ServiceAccountRole", opResult)
	if err != nil {
		return handleResultAndError(err)
	}

	// Intialize the deployment inputs required for the deployment resources
	r.logger.V(util.DebugLogLevel).Info("=== Initializing Deployment Configuration ===")
	deploymentConfig, err := r.initDeploymentConfiguration(ctx, hawtio, crNamespacedName)
	if err != nil {
		return handleResultAndError(err)
	}

	// Reconcile the configMap to ensure it is present for use with the deployment
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling ConfigMap ===")
	configMap, opResult, err := r.reconcileConfigMap(ctx, hawtio)
	r.logOperationResult("ConfigMap", opResult)
	if err != nil {
		return handleResultAndError(err)
	}

	// Makes the configMap available to the deployment
	r.logger.V(util.DebugLogLevel).Info(fmt.Sprintf("Assigning reconciled config map %s to deployment", crNamespacedName.Name))
	deploymentConfig.configMap = configMap

	// Reconcile the deployment resource
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling Deployment ===")
	opResult, err = r.reconcileDeployment(ctx, hawtio, deploymentConfig)
	r.logOperationResult("Deployment", opResult)
	if err != nil {
		return handleResultAndError(err)
	}

	// Reconcile the service resource
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling Service ===")
	opResult, err = r.reconcileService(ctx, hawtio)
	r.logOperationResult("Service", opResult)
	if err != nil {
		return handleResultAndError(err)
	}

	// Declare this for use later in the OAuthClient and Hawtio.Status
	var ingressRouteURL string

	// Reconcile the route resource, if applicable
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling Route ===")
	route, opResult, err := r.reconcileRoute(ctx, hawtio, deploymentConfig)
	r.logOperationResult("Route", opResult)
	if err != nil {
		return handleResultAndError(err)
	} else if route == nil && opResult != controllerutil.OperationResultNone {
		// This means the route was intentionally deleted to be regenerated.
		// Stop this loop and wait for the automatic requeue that the delete
		// event will trigger.
		r.logger.Info("Route was deleted for regeneration, ending this reconciliation loop.")
		return reconcile.Result{}, nil
	} else if route != nil {
		ingressRouteURL = oresources.GetRouteURL(route)
	}

	// Reconcile the ingress resource, if applicable
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling Ingress ===")
	ingress, opResult, err := r.reconcileIngress(ctx, hawtio, deploymentConfig)
	r.logOperationResult("Ingress", opResult)
	if err != nil {
		return handleResultAndError(err)
	} else if ingress != nil {
		ingressRouteURL = kresources.GetIngressURL(ingress)
	}

	// Reconcile the OAuthClient resource, if applicable
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling OAuth Client ===")
	opResult, err = r.reconcileOAuthClient(ctx, hawtio, ingressRouteURL, crNamespacedName)
	r.logOperationResult("OAuthClient", opResult)
	if err != nil {
		return handleResultAndError(err)
	}

	// Reconcile the ConsoleLink resource, if applicable
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling ConsoleLink ===")
	opResult, err = r.reconcileConsoleLink(ctx, hawtio, crNamespacedName, deploymentConfig, route)
	r.logOperationResult("ConsoleLink", opResult)
	if err != nil {
		return handleResultAndError(err)
	}

	// Reconcile the Certificate cronjob resource, if applicable
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling CronJob ===")
	opResult, err = r.reconcileCronJob(ctx, hawtio, crNamespacedName, opNamespacedName, deploymentConfig)
	r.logOperationResult("ConsoleLink", opResult)
	if err != nil {
		return handleResultAndError(err)
	}

	// =====================================================================
	// PHASE 5: UPDATE PHASE
	// =====================================================================
	r.logger.V(util.DebugLogLevel).Info("Update phase - refreshing Hawtio status")

	deployment := &appsv1.Deployment{}
	err = r.client.Get(ctx, crNamespacedName, deployment)
	if err != nil && kerrors.IsNotFound(err) {
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		r.logger.Error(err, "Failed to get deployment")
		return reconcile.Result{}, err
	}

	// Refresh the hawtio CR to minimize conflict window
	// Gets the absolute latest ResourceVersion from the server.
	if err := r.client.Get(ctx, crNamespacedName, hawtio); err != nil {
		return reconcile.Result{}, err
	}

	// Create a copy of the status to modify.
	newStatus := hawtio.Status.DeepCopy()

	// Reconcile status fields from the Deployment.
	newStatus.Replicas = deployment.Status.Replicas
	// Reconcile Hawtio status image field from deployment container image
	newStatus.Image = deployment.Spec.Template.Spec.Containers[0].Image
	newStatus.GatewayImage = deployment.Spec.Template.Spec.Containers[1].Image
	// Reconcile scale sub-resource labelSelectorPath from deployment spec to CR status
	if selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector); err == nil {
	   newStatus.Selector = selector.String()
	} else {
		return reconcile.Result{}, fmt.Errorf("failed to parse selector: %v", err)
	}

	r.logger.V(util.DebugLogLevel).Info("Adding Route/Ingress URL to Hawtio.Status.URL")

	if r.apiSpec.Routes && route != nil {
		// Reconcile route URL into Hawtio status
		newStatus.URL = ingressRouteURL
	} else if ingress != nil {
		newStatus.URL = ingressRouteURL
	}

	// Determine the overall phase based on the deployment's readiness.
	if deployment.Status.ReadyReplicas > 0 {
		newStatus.Phase = hawtiov2.HawtioPhaseDeployed
	} else {
		// The Deployment isn't ready yet. Let's check if it has timed out.
		if r.isDeploymentFailed(deployment) {
			newStatus.Phase = hawtiov2.HawtioPhaseFailed
		} else {
			// It's not ready, but it hasn't failed yet. It's likely pulling images
			// or waiting for the container to start. Keep it as Initialized.
			newStatus.Phase = hawtiov2.HawtioPhaseInitialized
		}
	}

	// Only send an update to the API server if the status has actually changed.
	// This prevents empty updates and reduces load on the API server.
	if !reflect.DeepEqual(hawtio.Status, *newStatus) {
		hawtio.Status = *newStatus
		r.logger.Info("Status has changed, updating Hawtio CR",
			"Phase", newStatus.Phase,
			"URL", newStatus.URL,
			"Replicas", newStatus.Replicas,
			"Image", newStatus.Image)
		if err := r.client.Status().Update(ctx, hawtio); err != nil {
			r.logger.Error(err, "Failed to update Hawtio status")
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}
