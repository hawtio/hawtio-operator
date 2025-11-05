package hawtio

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	errs "github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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

	consolev1 "github.com/openshift/api/console/v1"
	oauthv1 "github.com/openshift/api/oauth/v1"
	routev1 "github.com/openshift/api/route/v1"
	configclient "github.com/openshift/client-go/config/clientset/versioned"
	oauthclient "github.com/openshift/client-go/oauth/clientset/versioned"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	"github.com/hawtio/hawtio-operator/pkg/openshift"
	"github.com/hawtio/hawtio-operator/pkg/resources"
	kresources "github.com/hawtio/hawtio-operator/pkg/resources/kubernetes"
	oresources "github.com/hawtio/hawtio-operator/pkg/resources/openshift"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

var log = logf.Log.WithName("controller_hawtio")

const (
	hawtioFinalizer         = "hawt.io/finalizer"
	hostGeneratedAnnotation = "openshift.io/host.generated"
)

// Add creates a new Hawtio Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, bv util.BuildVariables) error {
	err := oauthv1.Install(mgr.GetScheme())
	if err != nil {
		return err
	}
	err = routev1.Install(mgr.GetScheme())
	if err != nil {
		return err
	}
	err = consolev1.Install(mgr.GetScheme())
	if err != nil {
		return err
	}
	err = apiextensionsv1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}

	r := &ReconcileHawtio{
		BuildVariables: bv,
		client:         mgr.GetClient(),
		scheme:         mgr.GetScheme(),
	}

	oauthClient, err := oauthclient.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.oauthClient = oauthClient

	configClient, err := configclient.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.configClient = configClient

	apiClient, err := kclient.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.apiClient = apiClient
	r.coreClient = apiClient.CoreV1()

	// Identify cluster capabilities
	r.apiSpec, err = capabilities.APICapabilities(context.TODO(), r.apiClient, r.configClient)
	if err != nil {
		return errs.Wrap(err, "Cluster API capability discovery failed")
	}

	if err := openshift.ConsoleYAMLSampleExists(); err == nil {
		openshift.CreateConsoleYAMLSamples(context.TODO(), mgr.GetClient(), r.ProductName)
	}

	return add(mgr, r, r.apiSpec.Routes)
}

func enqueueRequestForOwner[T client.Object](mgr manager.Manager) handler.TypedEventHandler[T, reconcile.Request] {
	return handler.TypedEnqueueRequestForOwner[T](mgr.GetScheme(), mgr.GetRESTMapper(), hawtiov2.NewHawtio(), handler.OnlyControllerOwner())
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler, routeSupport bool) error {
	// Create a new controller
	c, err := controller.New("hawtio-controller", mgr, controller.Options{Reconciler: r})
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

	if routeSupport {
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
					// Ignore updates to the Deployment other than the replicas one,
					// that are used to reconcile the Hawtio replicas.
					return oldDeployment.Status.Replicas != newDeployment.Status.Replicas
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

var _ reconcile.Reconciler = &ReconcileHawtio{}

// ReconcileHawtio reconciles a Hawtio object
type ReconcileHawtio struct {
	util.BuildVariables
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the API server
	client       client.Client
	scheme       *runtime.Scheme
	coreClient   corev1client.CoreV1Interface
	oauthClient  oauthclient.Interface
	configClient configclient.Interface
	apiClient    kclient.Interface
	apiSpec      *capabilities.ApiServerSpec
	logger       logr.Logger
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
	r.logger = log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	r.logger.Info("Reconciling Hawtio")

	r.logger.V(util.DebugLogLevel).Info(fmt.Sprintf("Cluster API Specification: %+v", r.apiSpec))

	// =====================================================================
	// PHASE 1: SETUP & FETCH
	// =====================================================================
	// Fetch the Hawtio instance from the cluster.
	r.logger.V(util.DebugLogLevel).Info("=== Fetching Hawtio Custom Resource ===")
	hawtio, err := r.fetchHawtio(ctx, request.NamespacedName)
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
	delete, err := r.handleDeletion(ctx, hawtio)
	if err != nil {
		return reconcile.Result{}, err
	} else if delete {
		return reconcile.Result{}, nil
	}

	// Aid deletion by adding finalizer
	r.logger.V(util.DebugLogLevel).Info("=== Add Finalizer ===")
	err = r.addFinalizer(ctx, hawtio)
	if err != nil {
		return reconcile.Result{}, err
	}

	// =====================================================================
	// PHASE 3: INITIALIZE STATUS
	// =====================================================================
	// If the status phase is empty, it's a new CR, so we initialize it.
	r.logger.V(util.DebugLogLevel).Info("=== Verify Hawtio Install Mode ===")
	specType, err := r.verifyHawtioSpecType(ctx, hawtio)
	if err != nil {
		return reconcile.Result{}, err
	} else if !specType {
		return reconcile.Result{Requeue: true}, err
	}

	// Check the status of the RBAC ConfigMap.
	// If specified in the CR then it should be present.
	r.logger.V(util.DebugLogLevel).Info("=== Verifying RBAC ConfigMap ===")
	valid, err := r.verifyRBACConfigMap(ctx, hawtio, request.NamespacedName)
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

	// Intialize the deployment inputs required for the deployment resources
	r.logger.V(util.DebugLogLevel).Info("=== Initializing Deployment Configuration ===")
	deploymentConfig, err := r.initDeploymentConfiguration(ctx, hawtio, request.NamespacedName)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the configMap to ensure it is present for use with the deployment
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling ConfigMap ===")
	configMap, opResult, err := r.reconcileConfigMap(ctx, hawtio)
	r.logOperationResult("ConfigMap", opResult)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Makes the configMap available to the deployment
	r.logger.V(util.DebugLogLevel).Info(fmt.Sprintf("Assigning reconciled config map %s to deployment", request.NamespacedName.Name))
	deploymentConfig.configMap = configMap

	// Reconcile the deployment resource
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling Deployment ===")
	opResult, err = r.reconcileDeployment(ctx, hawtio, deploymentConfig)
	r.logOperationResult("Deployment", opResult)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the service resource
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling Service ===")
	opResult, err = r.reconcileService(ctx, hawtio)
	r.logOperationResult("Service", opResult)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Declare this for use later in the OAuthClient and Hawtio.Status
	var ingressRouteURL string

	// Reconcile the route resource, if applicable
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling Route ===")
	route, opResult, err := r.reconcileRoute(ctx, hawtio, deploymentConfig)
	r.logOperationResult("Route", opResult)
	if err != nil {
		return reconcile.Result{}, err
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
		return reconcile.Result{}, err
	} else if ingress != nil {
		ingressRouteURL = kresources.GetIngressURL(ingress)
	}

	// Reconcile the service account as OAuth Client resource, if applicable
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling Service Account as OAuth Client ===")
	opResult, err = r.reconcileServiceAccountAsOauthClient(ctx, hawtio)
	r.logOperationResult("ServiceAccountAsOauthClient", opResult)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the OAuthClient resource, if applicable
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling OAuth Client ===")
	opResult, err = r.reconcileOAuthClient(ctx, hawtio, ingressRouteURL)
	r.logOperationResult("OAuthClient", opResult)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the ConsoleLink resource, if applicable
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling ConsoleLink ===")
	opResult, err = r.reconcileConsoleLink(ctx, hawtio, request.NamespacedName, deploymentConfig, route)
	r.logOperationResult("ConsoleLink", opResult)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the Certificate cronjob resource, if applicable
	r.logger.V(util.DebugLogLevel).Info("=== Reconciling CronJob ===")
	opResult, err = r.reconcileCronJob(ctx, hawtio, request.NamespacedName, deploymentConfig)
	r.logOperationResult("ConsoleLink", opResult)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconciling resources is complete. Set to deployed if not already
	if hawtio.Status.Phase != hawtiov2.HawtioPhaseDeployed {
		r.logger.V(util.DebugLogLevel).Info("Moving Hawtio.Status.Phase to deployed")
		err := r.setHawtioPhase(ctx, hawtio, hawtiov2.HawtioPhaseDeployed)
		if err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// =====================================================================
	// PHASE 5: UPDATE PHASE
	// =====================================================================
	r.logger.V(util.DebugLogLevel).Info("Update phase - refreshing Hawtio status")

	deployment := &appsv1.Deployment{}
	err = r.client.Get(ctx, request.NamespacedName, deployment)
	if err != nil && kerrors.IsNotFound(err) {
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		r.logger.Error(err, "Failed to get deployment")
		return reconcile.Result{}, err
	}

	// Create a copy of the status to modify.
	newStatus := hawtio.Status.DeepCopy()

	// Reconcile status fields from the Deployment.
	newStatus.Replicas = deployment.Status.Replicas
	// Reconcile Hawtio status image field from deployment container image
	newStatus.Image = deployment.Spec.Template.Spec.Containers[0].Image
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
		newStatus.Phase = hawtiov2.HawtioPhaseFailed
	}

	// Only send an update to the API server if the status has actually changed.
	// This prevents empty updates and reduces load on the API server.
	if !reflect.DeepEqual(hawtio.Status, *newStatus) {
		hawtio.Status = *newStatus
		r.logger.Info("Status has changed, updating Hawtio CR")
		if err := r.client.Status().Update(ctx, hawtio); err != nil {
			r.logger.Error(err, "Failed to update Hawtio status")
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileHawtio) fetchHawtio(ctx context.Context, namespacedName client.ObjectKey) (*hawtiov2.Hawtio, error) {
	r.logger.V(util.DebugLogLevel).Info("Fetching the Hawtio custom resource")

	hawtio := hawtiov2.NewHawtio()
	err := r.client.Get(ctx, namespacedName, hawtio)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			r.logger.V(util.DebugLogLevel).Info("No Hawtio CR found")
			return nil, nil
		}
		// Error reading the object - requeue the request.
		return nil, err
	}

	return hawtio, nil
}

func (r *ReconcileHawtio) handleDeletion(ctx context.Context, hawtio *hawtiov2.Hawtio) (bool, error) {
	if hawtio.GetDeletionTimestamp() == nil {
		return false, nil
	}

	r.logger.V(util.DebugLogLevel).Info("=== Deleting Installation ===")
	err := r.deletion(ctx, hawtio)
	if err != nil {
		return true, fmt.Errorf("deletion failed: %v", err)
	}
	// Deletion was successful (or is in progress),
	// return true to tell Reconcile() to stop.
	return true, nil
}

func (r *ReconcileHawtio) deletion(ctx context.Context, hawtio *hawtiov2.Hawtio) error {
	if controllerutil.ContainsFinalizer(hawtio, "foregroundDeletion") {
		return nil
	}

	if hawtio.Spec.Type == hawtiov2.ClusterHawtioDeploymentType {
		// Remove URI from OAuth client
		oc := &oauthv1.OAuthClient{}
		err := r.client.Get(ctx, types.NamespacedName{Name: resources.OAuthClientName}, oc)
		if err != nil && !kerrors.IsNotFound(err) {
			return fmt.Errorf("failed to get OAuth client: %v", err)
		}
		updated := resources.RemoveRedirectURIFromOauthClient(oc, hawtio.Status.URL)
		if updated {
			err := r.client.Update(ctx, oc)
			if err != nil {
				return fmt.Errorf("failed to remove redirect URI from OAuth client: %v", err)
			}
		}
	}

	// Remove OpenShift console link
	consoleLink := &consolev1.ConsoleLink{
		ObjectMeta: metav1.ObjectMeta{
			Name: hawtio.ObjectMeta.Name + "-" + hawtio.ObjectMeta.Namespace,
		},
	}
	err := r.client.Delete(ctx, consoleLink)
	if err != nil && !kerrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
		return fmt.Errorf("failed to delete console link: %v", err)
	}

	controllerutil.RemoveFinalizer(hawtio, hawtioFinalizer)
	err = r.client.Update(ctx, hawtio)
	if err != nil {
		return fmt.Errorf("failed to remove finalizer: %v", err)
	}

	return nil
}

func (r *ReconcileHawtio) addFinalizer(ctx context.Context, hawtio *hawtiov2.Hawtio) error {
	// Add a finalizer, that's needed to clean up cluster-wide resources, like ConsoleLink and OAuthClient
	r.logger.V(util.DebugLogLevel).Info("Adding a finalizer")
	if controllerutil.ContainsFinalizer(hawtio, hawtioFinalizer) {
		return nil
	}

	controllerutil.AddFinalizer(hawtio, hawtioFinalizer)
	err := r.client.Update(ctx, hawtio)
	if err != nil {
		return fmt.Errorf("failed to update finalizer: %v", err)
	}

	return nil
}

func (r *ReconcileHawtio) verifyHawtioSpecType(ctx context.Context, hawtio *hawtiov2.Hawtio) (bool, error) {
	if len(hawtio.Spec.Type) == 0 {
		r.logger.V(util.DebugLogLevel).Info("Hawtio.Spec.Type not specified. Defaulting to Cluster")
		hawtio.Spec.Type = hawtiov2.ClusterHawtioDeploymentType
		err := r.client.Update(ctx, hawtio)
		if err != nil {
			return false, fmt.Errorf("failed to update type: %v", err)
		}

		return false, nil
	}

	if hawtio.Spec.Type != hawtiov2.NamespaceHawtioDeploymentType && (hawtio.Spec.Type != hawtiov2.ClusterHawtioDeploymentType) {
		r.logger.V(util.DebugLogLevel).Info("Hawtio.Spec.Type neither Cluster or Namespace")

		err := r.setHawtioPhase(ctx, hawtio, hawtiov2.HawtioPhaseFailed)
		if err != nil {
			return false, err
		}

		err = fmt.Errorf("unsupported type: %s", hawtio.Spec.Type)
		return false, nil
	}

	return true, nil
}

func (r *ReconcileHawtio) verifyRBACConfigMap(ctx context.Context, hawtio *hawtiov2.Hawtio, namespacedName client.ObjectKey) (bool, error) {
	cm := hawtio.Spec.RBAC.ConfigMap
	if cm == "" {
		return true, nil // No RBAC configMap specified so default will be used
	}

	r.logger.V(util.DebugLogLevel).Info("Checking Hawtio.Spec.RBAC config map is valid")

	// Check that the ConfigMap exists
	var rbacConfigMap corev1.ConfigMap
	err := r.client.Get(ctx, types.NamespacedName{Namespace: namespacedName.Namespace, Name: cm}, &rbacConfigMap)
	if err != nil {
		r.logger.Error(err, "Failed to get RBAC ConfigMap")
		return false, err
	}

	if _, ok := rbacConfigMap.Data[resources.RBACConfigMapKey]; !ok {
		r.logger.Info("RBAC ConfigMap does not contain expected key: " + resources.RBACConfigMapKey, "ConfigMap", cm)
		// Let's poll for the RBAC ConfigMap to contain the expected key
		return false, nil
	}

	return true, nil
}

func (r *ReconcileHawtio) setHawtioPhase(ctx context.Context, hawtio *hawtiov2.Hawtio, phase hawtiov2.HawtioPhase) error {
	if hawtio.Status.Phase != phase {
		previous := hawtio.DeepCopy()
		hawtio.Status.Phase = phase
		err := r.client.Status().Patch(ctx, hawtio, client.MergeFrom(previous))
		if err != nil {
			return fmt.Errorf("failed to update hawtio phase to %s: %v", phase, err)
		}
	}

	return nil
}

func (r *ReconcileHawtio) initDeploymentConfiguration(ctx context.Context, hawtio *hawtiov2.Hawtio, namespacedName client.ObjectKey) (DeploymentConfiguration, error) {
	deploymentConfiguration := DeploymentConfiguration{}

	if r.apiSpec.IsOpenShift4 {
		//
		// === Find the OCP Console Public URL ===
		//
		r.logger.V(util.DebugLogLevel).Info("Setting console URL on deployment")

		cm, err := r.coreClient.ConfigMaps("openshift-config-managed").Get(ctx, "console-public", metav1.GetOptions{})
		if err != nil {
			if !kerrors.IsNotFound(err) && !kerrors.IsForbidden(err) {
				r.logger.Error(err, "Error getting OpenShift managed configuration")
				return deploymentConfiguration, err
			}
		} else {
			deploymentConfiguration.openShiftConsoleURL = cm.Data["consoleURL"]
		}

		r.logger.V(util.DebugLogLevel).Info("Creating OpenShift proxying certificate")

		//
		// === Create -proxying certificate - only applicable for OCP ===
		// === -serving certificate is automatically created on OCP ===
		//
		clientCertSecret, err := osCreateClientCertificate(ctx, r, hawtio, namespacedName.Name, namespacedName.Namespace)
		if err != nil {
			r.logger.Error(err, "Failed to create OpenShift proxying certificate")
			return deploymentConfiguration, err
		}
		deploymentConfiguration.clientCertSecret = clientCertSecret
	} else {
		//
		// === Create the Kubernetes serving certificate ===
		//
		r.logger.V(util.DebugLogLevel).Info("Creating Kubernetes serving certificate")

		// Create -serving certificate
		servingCertSecret, err := kubeCreateServingCertificate(ctx, r, hawtio, namespacedName.Name, namespacedName.Namespace)
		if err != nil {
			r.logger.Error(err, "Failed to create serving certificate")
			return deploymentConfiguration, err
		}
		deploymentConfiguration.servingCertSecret = servingCertSecret
	}

	//
	// === Custom Route certificate defined in Hawtio CR ===
	//
	if secretName := hawtio.Spec.Route.CertSecret.Name; secretName != "" {
		r.logger.V(util.DebugLogLevel).Info("Assigning Hawtio.Spec.Route certificate secret to deployment")
		deploymentConfiguration.tlsRouteSecret = &corev1.Secret{}
		err := r.client.Get(ctx, client.ObjectKey{Namespace: namespacedName.Namespace, Name: secretName}, deploymentConfiguration.tlsRouteSecret)
		if err != nil {
			return deploymentConfiguration, err
		}
	}

	//
	// === Custom Route CA certificate defined in Hawtio CR ===
	//
	if caCertSecretName := hawtio.Spec.Route.CaCert.Name; caCertSecretName != "" {
		r.logger.V(util.DebugLogLevel).Info("Assigning Hawtio.Spec.Route CA secret to deploment")
		deploymentConfiguration.caCertRouteSecret = &corev1.Secret{}
		err := r.client.Get(ctx, client.ObjectKey{Namespace: namespacedName.Namespace, Name: caCertSecretName}, deploymentConfiguration.caCertRouteSecret)
		if err != nil {
			return deploymentConfiguration, err
		}
	}

	return deploymentConfiguration, nil
}

func (r *ReconcileHawtio) reconcileConfigMap(ctx context.Context, hawtio *hawtiov2.Hawtio) (*corev1.ConfigMap, controllerutil.OperationResult, error) {
	configMap := resources.NewDefaultConfigMap(hawtio)

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, configMap, func() error {
		// Set the owner reference for garbage collection.
		if err := controllerutil.SetControllerReference(hawtio, configMap, r.scheme); err != nil {
			return err
		}

		// Get the target state for the ConfigMap
		reqLogger := log.WithName(fmt.Sprintf("%s-reconcileConfigMap", hawtio.Name))
		crConfigMap, err := resources.NewConfigMap(hawtio, r.apiSpec, reqLogger)
		if (err != nil) {
			reqLogger.Error(err, "Error reconciling ConfigMap")
			return err
		}

		// Mutate the object's Data field to match the desired state.
		configMap.Data = crConfigMap.Data

		return nil
	})

	if (err != nil) {
		return nil, opResult, err
	}

	return configMap, opResult, nil
}

func (r *ReconcileHawtio) reconcileDeployment(ctx context.Context, hawtio *hawtiov2.Hawtio, deploymentConfig DeploymentConfiguration) (controllerutil.OperationResult, error) {
	deployment := resources.NewDefaultDeployment(hawtio)

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, deployment, func() error {
		// Set the owner reference for garbage collection.
		if err := controllerutil.SetControllerReference(hawtio, deployment, r.scheme); err != nil {
			return err
		}

		clientCertSecretVersion := ""
		if deploymentConfig.clientCertSecret != nil {
			clientCertSecretVersion = deploymentConfig.clientCertSecret.GetResourceVersion()
		}

		reqLogger := log.WithName(fmt.Sprintf("%s-reconcileDeployment", hawtio.Name))
		crDeployment, err := resources.NewDeployment(hawtio, r.apiSpec,
													deploymentConfig.openShiftConsoleURL,
													deploymentConfig.configMap.GetResourceVersion(),
													clientCertSecretVersion,
													r.BuildVariables, reqLogger)
		if err != nil {
			reqLogger.Error(err, "Error reconciling deployment")
			return err
		}

		deployment.SetLabels(crDeployment.GetLabels())
		deployment.SetAnnotations(crDeployment.GetAnnotations())
		deployment.Spec = crDeployment.Spec
		return nil
	})

	return opResult, err
}

func (r *ReconcileHawtio) reconcileService(ctx context.Context, hawtio *hawtiov2.Hawtio) (controllerutil.OperationResult, error) {
	service := resources.NewDefaultService(hawtio)

	return controllerutil.CreateOrUpdate(ctx, r.client, service, func() error {
		// Set the owner reference for garbage collection.
		if err := controllerutil.SetControllerReference(hawtio, service, r.scheme); err != nil {
			return err
		}

		reqLogger := log.WithName(fmt.Sprintf("%s-reconcileService", hawtio.Name))
		crService := resources.NewService(hawtio, r.apiSpec, reqLogger)

		service.SetLabels(crService.GetLabels())
		service.SetAnnotations(crService.GetAnnotations())
		service.Spec = crService.Spec

		return nil
	})
}

func (r *ReconcileHawtio) reconcileRoute(ctx context.Context, hawtio *hawtiov2.Hawtio, deploymentConfig DeploymentConfiguration) (*routev1.Route, controllerutil.OperationResult, error) {
	if ! r.apiSpec.Routes {
		return nil, controllerutil.OperationResultNone, nil
	}

	existingRoute := &routev1.Route{}
	err := r.client.Get(ctx, types.NamespacedName{Name: hawtio.Name, Namespace: hawtio.Namespace}, existingRoute)

	if err == nil {
		// A route was found. Now, apply the special condition check.
		isGenerated := strings.EqualFold(existingRoute.Annotations[hostGeneratedAnnotation], "true")

		if hawtio.Spec.RouteHostName == "" && !isGenerated {
			// The user cleared the hostname, and the current route is not auto-generated.
			//
			// Emptying route host is ignored so it's not possible to re-generate the host
			// See https://github.com/openshift/origin/pull/9425
			// We must delete the route to force a regeneration.

			r.logger.Info("Deleting Route to trigger hostname regeneration.", "Route.Name", existingRoute.Name)
			if err := r.client.Delete(ctx, existingRoute); err != nil {
				r.logger.Error(err, "Failed to delete Route for regeneration")
				return nil, controllerutil.OperationResultNone, err
			}

			// Deletion was successful. We must stop this reconciliation loop here.
			// The next loop will find the Route is missing and will create a new one.
			// Returning (nil, nil) signals success for this loop, allowing the next one to proceed cleanly.
			return nil, controllerutil.OperationResultUpdated, nil
		}
	} else if !kerrors.IsNotFound(err) {
		// A real error occurred trying to get the Route. Fail fast.
		log.Error(err, "Failed to get existing Route for pre-check")
		return nil, controllerutil.OperationResultNone, err
	}

	// err was not found so carry-on with creating a new route
	route := oresources.NewDefaultRoute(hawtio)

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, route, func() error {
		// Set the owner reference for garbage collection.
		if err := controllerutil.SetControllerReference(hawtio, route, r.scheme); err != nil {
			return err
		}

		reqLogger := log.WithName(fmt.Sprintf("%s-reconcileRoute", hawtio.Name))
		crRoute := oresources.NewRoute(hawtio, deploymentConfig.tlsRouteSecret, deploymentConfig.caCertRouteSecret, reqLogger)

		route.SetLabels(crRoute.GetLabels())
		route.SetAnnotations(crRoute.GetAnnotations())
		route.Spec = crRoute.Spec

		return nil
	})

	if err != nil {
		return nil, opResult, err
	}

	return route, opResult, nil
}

func (r *ReconcileHawtio) reconcileIngress(ctx context.Context, hawtio *hawtiov2.Hawtio, deploymentConfig DeploymentConfiguration) (*networkingv1.Ingress, controllerutil.OperationResult, error) {
	if r.apiSpec.Routes {
		return nil, controllerutil.OperationResultNone, nil
	}

	ingress := kresources.NewDefaultIngress(hawtio)

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, ingress, func() error {
		// Set the owner reference for garbage collection.
		if err := controllerutil.SetControllerReference(hawtio, ingress, r.scheme); err != nil {
			return err
		}

		reqLogger := log.WithName(fmt.Sprintf("%s-reconcileIngress", hawtio.Name))
		crIngress := kresources.NewIngress(hawtio, r.apiSpec, deploymentConfig.servingCertSecret, reqLogger)

		ingress.SetLabels(crIngress.GetLabels())
		ingress.SetAnnotations(crIngress.GetAnnotations())
		ingress.Spec = crIngress.Spec

		return nil
	})

	if err != nil {
		return nil, opResult, err
	}

	return ingress, opResult, nil
}

func (r *ReconcileHawtio) reconcileServiceAccountAsOauthClient(ctx context.Context, hawtio *hawtiov2.Hawtio) (controllerutil.OperationResult, error) {
	if hawtio.Spec.Type != hawtiov2.NamespaceHawtioDeploymentType {
		return controllerutil.OperationResultNone, nil
	}

	serviceAccount := resources.NewDefaultServiceAccountAsOauthClient(hawtio)

	return controllerutil.CreateOrUpdate(ctx, r.client, serviceAccount, func() error {
		// Set the owner reference for garbage collection.
		if err := controllerutil.SetControllerReference(hawtio, serviceAccount, r.scheme); err != nil {
			return err
		}

		crServiceAccount, err := resources.NewServiceAccountAsOauthClient(hawtio)
		if (err != nil) {
			return err
		}

		serviceAccount.SetLabels(crServiceAccount.GetLabels())
		serviceAccount.SetAnnotations(crServiceAccount.GetAnnotations())
		return nil
	})
}

func (r *ReconcileHawtio) reconcileOAuthClient(ctx context.Context, hawtio *hawtiov2.Hawtio, newRouteURL string)  (controllerutil.OperationResult, error) {
	if !r.apiSpec.IsOpenShift4 {
		// Not applicable to cluster
		return controllerutil.OperationResultNone, nil
	}

	shouldExist := hawtio.Spec.Type == hawtiov2.ClusterHawtioDeploymentType

	// Should the OAuthClient exist at all?
	if !shouldExist {
		// The CR is not cluster-scoped, so we must ensure the OAuthClient is cleaned up.
		// Note: We use the direct client here to handle potential permission issues.
		existingOAuthClient := &oauthv1.OAuthClient{}
		err := r.client.Get(ctx, types.NamespacedName{Name: resources.OAuthClientName}, existingOAuthClient)
		if err != nil {
			if kerrors.IsNotFound(err) {
				return controllerutil.OperationResultNone, nil // Already gone, which is correct.
			}
			// If we get a Forbidden error, we assume we can't manage it anyway.
			if kerrors.IsForbidden(err) {
				r.logger.Info("Operator is not permitted to clean up cluster OAuthClient; skipping.")
				return controllerutil.OperationResultNone, nil
			}
			return controllerutil.OperationResultNone, err
		}

		// Found an existing OAuthClient, let's remove our URI from it.
		r.logger.Info("Hawtio is not cluster-scoped, removing RedirectURI from OAuthClient")
		if resources.RemoveRedirectURIFromOauthClient(existingOAuthClient, newRouteURL) {
			err := r.client.Update(ctx, existingOAuthClient)
			return controllerutil.OperationResultUpdated, err
		}

		return controllerutil.OperationResultNone, nil
	}

	// Ensure the OAuthClient exists and is correctly configured.
	oAuthClient := resources.NewDefaultOAuthClient(resources.OAuthClientName)

	// We use CreateOrUpdate to ensure the base object exists.
	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, oAuthClient, func() error {
		crOAuthClient := resources.NewOAuthClient(resources.OAuthClientName)
		oAuthClient.GrantMethod = crOAuthClient.GrantMethod
		oAuthClient.RedirectURIs = crOAuthClient.RedirectURIs
		return nil
	})

	if err != nil {
		return controllerutil.OperationResultNone, err
	}

	// Read-Modify-Write for RedirectURIs
	// We must re-fetch the object to ensure we have the latest version
	// before modifying the list.
	err = r.client.Get(ctx, types.NamespacedName{Name: resources.OAuthClientName}, oAuthClient)
	if err != nil {
		return controllerutil.OperationResultNone, err
	}

	updateOAuthClient := false
	oldRouteURL := hawtio.Status.URL
	// Remove the old URL if it's different from the new one
	if oldRouteURL != "" && oldRouteURL != newRouteURL {
		r.logger.Info("Removing stale RedirectURI from OAuthClient", "URI", oldRouteURL)
		if resources.RemoveRedirectURIFromOauthClient(oAuthClient, oldRouteURL) {
			updateOAuthClient = true
		}
	}

	// Add the current route URL if it's not already present.
	if ok, _ := resources.OauthClientContainsRedirectURI(oAuthClient, newRouteURL); !ok && newRouteURL != "" {
		r.logger.Info("Adding new RedirectURI to OAuthClient", "URI", newRouteURL)
		oAuthClient.RedirectURIs = append(oAuthClient.RedirectURIs, newRouteURL)
		updateOAuthClient = true
	}

	if updateOAuthClient {
		err := r.client.Update(ctx, oAuthClient)
		return controllerutil.OperationResultUpdated, err
	}

	return opResult, nil
}

func (r *ReconcileHawtio) removeConsoleLink(ctx context.Context, consoleLinkName string) (controllerutil.OperationResult, error) {
	consoleLink := &consolev1.ConsoleLink{}
	err := r.client.Get(ctx, types.NamespacedName{Name: consoleLinkName}, consoleLink)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// The link doesn't exist, which is the desired state. Nothing to do.
			return controllerutil.OperationResultNone, nil
		}
		// A real error occurred trying to get the object.
		return controllerutil.OperationResultNone, err
	}

	// If we get here, we found a stale ConsoleLink that needs to be deleted.
	r.logger.Info("Deleting stale ConsoleLink", "ConsoleLink.Name", consoleLinkName)
	if err := r.client.Delete(ctx, consoleLink); err != nil {
		return controllerutil.OperationResultNone, err
	}

	return controllerutil.OperationResultUpdated, nil
}

func (r *ReconcileHawtio) reconcileConsoleLink(ctx context.Context, hawtio *hawtiov2.Hawtio, namespacedName client.ObjectKey, deploymentConfig DeploymentConfiguration, route *routev1.Route) (controllerutil.OperationResult, error) {
	// If not OpenShift 4, ConsoleLink is irrelevant. Do nothing.
	if !r.apiSpec.IsOpenShift4 {
		log.V(util.DebugLogLevel).Info("Not an OpenShift 4 cluster, skipping ConsoleLink reconciliation.")
		return controllerutil.OperationResultNone, nil
	}

	consoleLinkName := namespacedName.Name + "-" + namespacedName.Namespace

	// The only prerequisites are being on OCP and having a valid Route.
	shouldExist := r.apiSpec.Routes && route != nil && route.Spec.Host != ""

	// Prerequisite check
	r.logger.V(util.DebugLogLevel).Info("Reconcile ConsoleLink - Prerequisite Check")
	if !shouldExist {
		r.logger.V(util.DebugLogLevel).Info("Removing ConsoleLink as not required")
		return r.removeConsoleLink(ctx, consoleLinkName)
	}

	r.logger.V(util.DebugLogLevel).Info("Reconcile ConsoleLink - Retrieving HawtConfig")
	hawtconfig, err := resources.GetHawtioConfig(deploymentConfig.configMap)
	if err != nil {
		r.logger.Error(err, "Failed to get hawtconfig")
		return controllerutil.OperationResultNone, err
	}

	r.logger.V(util.DebugLogLevel).Info("Reconcile ConsoleLink - Creating new ConsoleLink")
	consoleLink := &consolev1.ConsoleLink{
		ObjectMeta: metav1.ObjectMeta{
			Name: consoleLinkName,
		},
	}

	return controllerutil.CreateOrUpdate(ctx, r.client, consoleLink, func() error {
		var crConsoleLink *consolev1.ConsoleLink

		if hawtio.Spec.Type == hawtiov2.ClusterHawtioDeploymentType {
			r.logger.V(util.DebugLogLevel).Info("Adding console link as Application Menu Link")
			crConsoleLink = openshift.NewApplicationMenuLink(consoleLinkName, route, hawtconfig)
		} else if r.apiSpec.IsOpenShift43Plus {
			r.logger.V(util.DebugLogLevel).Info("Adding console link as Namespace Dashboard Link")
			crConsoleLink = openshift.NewNamespaceDashboardLink(consoleLinkName, namespacedName.Namespace, route, hawtconfig)
		}  else {
			// If no link should exist, we can't model that with CreateOrUpdate.
			// This case is handled below. For the mutate function, we do nothing.
			return nil
		}

		consoleLink.Spec = crConsoleLink.Spec
		consoleLink.SetLabels(crConsoleLink.GetLabels())
		consoleLink.SetAnnotations(crConsoleLink.GetAnnotations())

		return nil
	})
}

func (r *ReconcileHawtio) reconcileCronJob(ctx context.Context, hawtio *hawtiov2.Hawtio, namespacedName client.ObjectKey, deploymentConfig DeploymentConfiguration) (controllerutil.OperationResult, error) {
	if deploymentConfig.clientCertSecret == nil {
		// No certificate so cronjob not necessary
		return controllerutil.OperationResultNone, nil
	}

	// Determine if the CronJob should exist.
	shouldExist := hawtio.Spec.Auth.ClientCertCheckSchedule != ""
	cronJobName := hawtio.Name + "-certificate-expiry-check"

	if shouldExist {
		// The CronJob SHOULD exist. ---
		r.logger.Info("Ensuring CronJob exists and is up to date", "CronJob.Name", cronJobName)
		cronJob := resources.NewDefaultCronJob(hawtio)

		return controllerutil.CreateOrUpdate(ctx, r.client, cronJob, func() error {
			// Set the owner reference for garbage collection.
			if err := controllerutil.SetControllerReference(hawtio, cronJob, r.scheme); err != nil {
				return err
			}

			pod, err := getOperatorPod(ctx, r.client, namespacedName.Namespace)
			if err != nil {
				return err
			}

			crCronJob, err := resources.NewCronJob(hawtio, pod, namespacedName.Namespace)
			if err != nil {
				return fmt.Errorf("failed to build desired cronjob: %w", err)
			}

			cronJob.Spec = crCronJob.Spec

			return nil
		})

	} else {
		// The CronJob SHOULD NOT exist. ---
		// We must ensure it is deleted if it's found.
		log.V(util.DebugLogLevel).Info("Ensuring CronJob does not exist", "CronJob.Name", cronJobName)

		staleCronJob := &batchv1.CronJob{}
		err := r.client.Get(ctx, types.NamespacedName{Name: cronJobName, Namespace: hawtio.Namespace}, staleCronJob)

		if err != nil {
			if kerrors.IsNotFound(err) {
				// It doesn't exist, which is what we want. Success.
				return controllerutil.OperationResultNone, nil
			}
			return controllerutil.OperationResultNone, err // A real error occurred.
		}

		// If we found it, it's a stale resource that needs to be deleted.
		log.Info("Deleting stale CronJob", "CronJob.Name", cronJobName)
		if err := r.client.Delete(ctx, staleCronJob); err != nil {
			return controllerutil.OperationResultNone, err
		}

		return controllerutil.OperationResultUpdated, nil // Signifies a change (deletion) was made.
	}
}

func (r *ReconcileHawtio) logOperationResult(resource string, result controllerutil.OperationResult) {
	if result == controllerutil.OperationResultNone {
		return // no need to log occasions where no action was taken
	}

	r.logger.Info("=== Resource "+ resource + " Reconciliation Completed ===", "Result", result)
}

func getOperatorPod(ctx context.Context, c client.Client, namespace string) (*corev1.Pod, error) {
	podName, _ := os.LookupEnv("POD_NAME")

	pod := &corev1.Pod{}
	err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: podName}, pod)

	if err != nil {
		log.Error(err, "Pod not found")
		return nil, err
	}
	return pod, nil
}
