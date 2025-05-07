package hawtio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/RHsyseng/operator-utils/pkg/resource/compare"
	"github.com/RHsyseng/operator-utils/pkg/resource/read"
	"github.com/RHsyseng/operator-utils/pkg/resource/write"
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
	hawtioFinalizer         = "finalizer.hawtio.hawt.io"
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
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Hawtio")

	// Fetch the Hawtio instance
	reqLogger.V(util.DebugLogLevel).Info("Fetching the Hawtio custom resource")

	hawtio := hawtiov2.NewHawtio()
	err := r.client.Get(ctx, request.NamespacedName, hawtio)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	deploymentConfig := DeploymentConfiguration{}

	reqLogger.V(util.DebugLogLevel).Info(fmt.Sprintf("Cluster API Specification: %+v", r.apiSpec))

	// Delete phase

	if hawtio.GetDeletionTimestamp() != nil {
		err = r.deletion(ctx, hawtio)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("deletion failed: %v", err)
		}
		return reconcile.Result{}, nil
	}

	// Add a finalizer, that's needed to clean up cluster-wide resources, like ConsoleLink and OAuthClient
	reqLogger.V(util.DebugLogLevel).Info("Adding a finalizer")
	if !controllerutil.ContainsFinalizer(hawtio, hawtioFinalizer) {
		controllerutil.AddFinalizer(hawtio, hawtioFinalizer)
		err = r.client.Update(ctx, hawtio)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update finalizer: %v", err)
		}
	}

	// Init phase

	if len(hawtio.Spec.Type) == 0 {
		reqLogger.V(util.DebugLogLevel).Info("Hawtio.Spec.Type not specified. Defaulting to Cluster")
		hawtio.Spec.Type = hawtiov2.ClusterHawtioDeploymentType
		err = r.client.Update(ctx, hawtio)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update type: %v", err)
		}
		return reconcile.Result{}, nil
	}

	// Invariant checks
	isClusterDeployment := hawtio.Spec.Type == hawtiov2.ClusterHawtioDeploymentType
	isNamespaceDeployment := hawtio.Spec.Type == hawtiov2.NamespaceHawtioDeploymentType

	if !isNamespaceDeployment && !isClusterDeployment {
		reqLogger.V(util.DebugLogLevel).Info("Hawtio.Spec.Type neither Cluster or Namespace")

		err := fmt.Errorf("unsupported type: %s", hawtio.Spec.Type)
		if hawtio.Status.Phase != hawtiov2.HawtioPhaseFailed {
			previous := hawtio.DeepCopy()
			hawtio.Status.Phase = hawtiov2.HawtioPhaseFailed
			err = r.client.Status().Patch(ctx, hawtio, client.MergeFrom(previous))
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to update phase: %v", err)
			}
		}
		return reconcile.Result{}, err
	}

	if len(hawtio.Status.Phase) == 0 || hawtio.Status.Phase == hawtiov2.HawtioPhaseFailed {
		reqLogger.V(util.DebugLogLevel).Info("Hawtio.Status.Phase is zero or failed")

		previous := hawtio.DeepCopy()
		hawtio.Status.Phase = hawtiov2.HawtioPhaseInitialized
		err = r.client.Status().Patch(ctx, hawtio, client.MergeFrom(previous))
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update phase: %v", err)
		}
		return reconcile.Result{Requeue: true}, nil
	}

	if r.apiSpec.IsOpenShift4 {
		reqLogger.V(util.DebugLogLevel).Info("Setting console URL on deployment")

		// Retrieve OpenShift Web console public URL
		cm, err := r.coreClient.ConfigMaps("openshift-config-managed").Get(ctx, "console-public", metav1.GetOptions{})
		if err != nil {
			if !kerrors.IsNotFound(err) && !kerrors.IsForbidden(err) {
				reqLogger.Error(err, "Error getting OpenShift managed configuration")
				return reconcile.Result{}, err
			}
		} else {
			deploymentConfig.openShiftConsoleURL = cm.Data["consoleURL"]
		}
	}

	if r.apiSpec.IsOpenShift4 {
		reqLogger.V(util.DebugLogLevel).Info("Creating OpenShift proxying certificate")

		// Create -proxying certificate
		// -serving certificate is automatically created
		clientCertSecret, err := osCreateClientCertificate(ctx, r, hawtio, request.Name, request.Namespace)
		if err != nil {
			reqLogger.Error(err, "Failed to create OpenShift proxying certificate")
			return reconcile.Result{}, err
		}
		deploymentConfig.clientCertSecret = clientCertSecret
	} else {
		reqLogger.V(util.DebugLogLevel).Info("Creating Kubernetes serving certificate")

		// Create -serving certificate
		servingCertSecret, err := kubeCreateServingCertificate(ctx, r, hawtio, request.Name, request.Namespace)
		if err != nil {
			reqLogger.Error(err, "Failed to create serving certificate")
			return reconcile.Result{}, err
		}
		deploymentConfig.servingCertSecret = servingCertSecret
	}

	//
	// Custom Route certificates defined in Hawtio CR
	//
	if secretName := hawtio.Spec.Route.CertSecret.Name; secretName != "" {
		reqLogger.V(util.DebugLogLevel).Info("Assigning Hawtio.Spec.Route certificate secret to deployment")
		deploymentConfig.tlsRouteSecret = &corev1.Secret{}
		err = r.client.Get(ctx, client.ObjectKey{Namespace: request.Namespace, Name: secretName}, deploymentConfig.tlsRouteSecret)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	if caCertSecretName := hawtio.Spec.Route.CaCert.Name; caCertSecretName != "" {
		reqLogger.V(util.DebugLogLevel).Info("Assigning Hawtio.Spec.Route CA secret to deploment")
		deploymentConfig.caCertRouteSecret = &corev1.Secret{}
		err = r.client.Get(ctx, client.ObjectKey{Namespace: request.Namespace, Name: caCertSecretName}, deploymentConfig.caCertRouteSecret)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	if cm := hawtio.Spec.RBAC.ConfigMap; cm != "" {
		reqLogger.V(util.DebugLogLevel).Info("Checking Hawtio.Spec.RBAC config map is valid")

		// Check that the ConfigMap exists
		var rbacConfigMap corev1.ConfigMap
		err := r.client.Get(ctx, types.NamespacedName{Namespace: request.Namespace, Name: cm}, &rbacConfigMap)
		if err != nil {
			if kerrors.IsNotFound(err) {
				reqLogger.Info("RBAC ConfigMap must be created", "ConfigMap", cm)
				// Let's poll for the RBAC ConfigMap to be created
				return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
			} else {
				reqLogger.Error(err, "Failed to get RBAC ConfigMap")
				return reconcile.Result{}, err
			}
		}
		if _, ok := rbacConfigMap.Data[resources.RBACConfigMapKey]; !ok {
			reqLogger.Info("RBAC ConfigMap does not contain expected key: "+resources.RBACConfigMapKey, "ConfigMap", cm)
			// Let's poll for the RBAC ConfigMap to contain the expected key
			return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
		}
	}

	_, err = r.reconcileConfigMap(hawtio)
	if err != nil {
		reqLogger.Error(err, "Error reconciling ConfigMap")
		return reconcile.Result{}, err
	}

	reqLogger.V(util.DebugLogLevel).Info(fmt.Sprintf("Assigning config map %s to deployment", request.NamespacedName.Name))
	deploymentConfig.configMap = &corev1.ConfigMap{}
	err = r.client.Get(ctx, request.NamespacedName, deploymentConfig.configMap)
	if err != nil {
		reqLogger.Error(err, "Failed to get config map")
		return reconcile.Result{}, err
	}

	_, err = r.reconcileDeployment(hawtio, deploymentConfig)
	if err != nil {
		reqLogger.Error(err, "Error reconciling deployment")
		return reconcile.Result{}, err
	}

	var ingress *networkingv1.Ingress
	var route *routev1.Route
	if r.apiSpec.Routes {
		reqLogger.V(util.DebugLogLevel).Info(fmt.Sprintf("Checking Route %s", request.NamespacedName.Name))

		route = &routev1.Route{}
		err = r.client.Get(ctx, request.NamespacedName, route)
		if err != nil && kerrors.IsNotFound(err) {
			return reconcile.Result{Requeue: true}, nil
		} else if err != nil {
			reqLogger.Error(err, "Failed to get route")
			return reconcile.Result{}, err
		}

		if route == nil {
			err := errors.New("Route could not be found")
			reqLogger.Error(err, "Route failure")
			return reconcile.Result{}, err
		}
	} else {
		reqLogger.V(util.DebugLogLevel).Info(fmt.Sprintf("Checking Ingress %s", request.NamespacedName.Name))

		ingress = &networkingv1.Ingress{}
		err = r.client.Get(ctx, request.NamespacedName, ingress)
		if err != nil && kerrors.IsNotFound(err) {
			return reconcile.Result{Requeue: true}, nil
		} else if err != nil {
			reqLogger.Error(err, "Failed to get ingress")
			return reconcile.Result{}, err
		}

		if ingress == nil {
			err := errors.New("Ingress could not be found")
			reqLogger.Error(err, "Ingress failure")
			return reconcile.Result{}, err
		}
	}

	if r.apiSpec.IsOpenShift4 && isClusterDeployment {
		reqLogger.V(util.DebugLogLevel).Info("Creating OAuth client for OpenShift Cluster deployment")

		// Add OAuth client
		oauthClient := resources.NewOAuthClient(resources.OAuthClientName)
		err = r.client.Create(ctx, oauthClient)
		if err != nil && !kerrors.IsAlreadyExists(err) {
			return reconcile.Result{}, err
		}
	}

	// Read Hawtio configuration
	reqLogger.V(util.DebugLogLevel).Info("Reading Hawtio configuration from deployment config map")
	hawtconfig, err := resources.GetHawtioConfig(deploymentConfig.configMap)
	if err != nil {
		reqLogger.Error(err, "Failed to get hawtconfig")
		return reconcile.Result{}, err
	}

	// Add link to OpenShift console
	consoleLinkName := request.Name + "-" + request.Namespace
	if r.apiSpec.IsOpenShift4 && r.apiSpec.Routes && hawtio.Status.Phase == hawtiov2.HawtioPhaseInitialized {
		reqLogger.V(util.DebugLogLevel).Info(fmt.Sprintf("Adding console link %s", consoleLinkName))

		// With checks above, route should not be null
		consoleLink := &consolev1.ConsoleLink{}
		err = r.client.Get(ctx, types.NamespacedName{Name: consoleLinkName}, consoleLink)
		if err != nil {
			if !kerrors.IsNotFound(err) {
				reqLogger.Error(err, "Failed to get console link")
				return reconcile.Result{}, err
			}
		} else {
			err = r.client.Delete(ctx, consoleLink)
			if err != nil {
				reqLogger.Error(err, "Failed to delete console link")
				return reconcile.Result{}, err
			}
		}

		consoleLink = &consolev1.ConsoleLink{}
		if isClusterDeployment {
			reqLogger.V(util.DebugLogLevel).Info("Adding console link as Application Menu Link")
			consoleLink = openshift.NewApplicationMenuLink(consoleLinkName, route, hawtconfig)
		} else if r.apiSpec.IsOpenShift43Plus {
			reqLogger.V(util.DebugLogLevel).Info("Adding console link as Namespace Dashboard Link")
			consoleLink = openshift.NewNamespaceDashboardLink(consoleLinkName, request.Namespace, route, hawtconfig)
		}
		if consoleLink.Spec.Location != "" {
			err = r.client.Create(ctx, consoleLink)
			if err != nil {
				reqLogger.Error(err, "Failed to create console link", "name", consoleLink.Name)
				return reconcile.Result{}, err
			}
		}
	}

	// Update status
	if hawtio.Status.Phase != hawtiov2.HawtioPhaseDeployed {
		reqLogger.V(util.DebugLogLevel).Info("Moving Hawtio.Status.Phase to deployed")

		previous := hawtio.DeepCopy()
		hawtio.Status.Phase = hawtiov2.HawtioPhaseDeployed
		err = r.client.Status().Patch(ctx, hawtio, client.MergeFrom(previous))
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update phase: %v", err)
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Update phase
	reqLogger.V(util.DebugLogLevel).Info("Reconciling in Update phase")

	hawtioCopy := hawtio.DeepCopy()

	deployment := &appsv1.Deployment{}
	err = r.client.Get(ctx, request.NamespacedName, deployment)
	if err != nil && kerrors.IsNotFound(err) {
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get deployment")
		return reconcile.Result{}, err
	}

	// Reconcile replicas into Hawtio status
	hawtioCopy.Status.Replicas = deployment.Status.Replicas

	// Reconcile scale sub-resource labelSelectorPath from deployment spec to CR status
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to parse selector: %v", err)
	}
	hawtioCopy.Status.Selector = selector.String()

	// Reconcile Hawtio status image field from deployment container image
	hawtioCopy.Status.Image = deployment.Spec.Template.Spec.Containers[0].Image

	reqLogger.V(util.DebugLogLevel).Info("Adding Route/Ingress URL to Hawtio.Status.URL")
	var ingressRouteURL string
	if r.apiSpec.Routes {
		// With checks above, route should not be null

		// Reconcile route URL into Hawtio status
		ingressRouteURL = oresources.GetRouteURL(route)
		hawtioCopy.Status.URL = ingressRouteURL

		// Reconcile route host from routeHostName field
		if hostName := hawtio.Spec.RouteHostName; len(hostName) == 0 && !strings.EqualFold(route.Annotations[hostGeneratedAnnotation], "true") {
			// Emptying route host is ignored so it's not possible to re-generate the host
			// See https://github.com/openshift/origin/pull/9425
			// In that case, let's delete the route
			err := r.client.Delete(ctx, route)
			if err != nil {
				reqLogger.Error(err, "Failed to delete route to auto-generate hostname")
				return reconcile.Result{}, err
			}
			// And requeue to create a new route in the next reconcile loop
			return reconcile.Result{Requeue: true}, nil
		}
	} else {
		ingressRouteURL = kresources.GetIngressURL(ingress)
		hawtioCopy.Status.URL = ingressRouteURL
	}

	// Reconcile console link in OpenShift console
	if r.apiSpec.IsOpenShift4 && r.apiSpec.Routes {
		// With checks above, route should not be null
		reqLogger.V(util.DebugLogLevel).Info(fmt.Sprintf("Reconciling console link %s", consoleLinkName))

		consoleLink := &consolev1.ConsoleLink{}
		err = r.client.Get(ctx, types.NamespacedName{Name: consoleLinkName}, consoleLink)
		if err != nil {
			if kerrors.IsNotFound(err) {
				// If not found, create a console link
				if isClusterDeployment {
					consoleLink = openshift.NewApplicationMenuLink(consoleLinkName, route, hawtconfig)
				} else if r.apiSpec.IsOpenShift43Plus {
					consoleLink = openshift.NewNamespaceDashboardLink(consoleLinkName, request.Namespace, route, hawtconfig)
				}
				if consoleLink.Spec.Location != "" {
					err = r.client.Create(ctx, consoleLink)
					if err != nil {
						reqLogger.Error(err, "Failed to create console link", "name", consoleLink.Name)
						return reconcile.Result{}, err
					}
				}
			} else {
				reqLogger.Error(err, "Failed to get console link")
				return reconcile.Result{}, err
			}
		} else {
			consoleLinkCopy := consoleLink.DeepCopy()
			if isClusterDeployment {
				reqLogger.V(util.DebugLogLevel).Info("Updating console link as Application Menu Link")
				openshift.UpdateApplicationMenuLink(consoleLinkCopy, route, hawtconfig)
			} else if r.apiSpec.IsOpenShift43Plus {
				reqLogger.V(util.DebugLogLevel).Info("Updating console link as Namespace Dashboard Link")
				openshift.UpdateNamespaceDashboardLink(consoleLinkCopy, route, hawtconfig)
			}
			err = r.client.Patch(ctx, consoleLinkCopy, client.MergeFrom(consoleLink))
			if err != nil {
				reqLogger.Error(err, "Failed to update console link", "name", consoleLink.Name)
				return reconcile.Result{}, err
			}
		}
	}

	// Reconcile the client certificate cronJob
	cronJob := &batchv1.CronJob{}
	cronJobName := request.Name + "-certificate-expiry-check"

	reqLogger.V(util.DebugLogLevel).Info("Reconciling cronjob for certificate expiry checking")
	if cronJobErr := r.client.Get(ctx, client.ObjectKey{Namespace: request.Namespace, Name: cronJobName}, cronJob); cronJobErr == nil {
		update := false
		if hawtio.Spec.Auth.ClientCertCheckSchedule != "" {
			if cronJob.Spec.Schedule != hawtio.Spec.Auth.ClientCertCheckSchedule {
				cronJob.Spec.Schedule = hawtio.Spec.Auth.ClientCertCheckSchedule
				update = true
			}
			updateExp, err := updateExpirationPeriod(cronJob, hawtio.Spec.Auth.ClientCertExpirationPeriod)
			if err != nil {
				log.Error(err, "CronJob haven't been updated")
			}

			if update || updateExp {
				err = r.client.Update(ctx, cronJob)

				if err != nil {
					log.Error(err, "CronJob haven't been updated")
				}
			}

			//if cronjob exists and ClientCertRotate is disabled, cronjob has to be deleted
		} else {
			err = r.client.Delete(ctx, cronJob)
			if err != nil {
				log.Error(err, "CronJob could not be deleted")
			}
		}
	}

	/*
	 * Reconcile OAuth client
	 * Do not use the default client whose cached informers require
	 * permission to list cluster wide oauth clients
	 */
	if r.apiSpec.IsOpenShift4 {
		reqLogger.V(util.DebugLogLevel).Info("Reconciling OAuth client on openshift cluster")
		oc, err := r.oauthClient.OauthV1().OAuthClients().Get(ctx, resources.OAuthClientName, metav1.GetOptions{})
		if err != nil {
			if kerrors.IsNotFound(err) {
				// OAuth client should not be found for namespace deployment type
				// except when it changes from "cluster" to "namespace"
				if isClusterDeployment {
					return reconcile.Result{Requeue: true}, nil
				}
			} else if !(kerrors.IsForbidden(err) && isNamespaceDeployment) {
				// We tolerate 403 for namespace deployment as the operator
				// may not have permission to read cluster wide resources
				// like OAuth clients
				reqLogger.Error(err, "Failed to get OAuth client")
				return reconcile.Result{}, err
			}
		}

		// TODO: OAuth client reconciliation triggered by roll-out deployment should ideally
		// wait until the deployment is successful before deleting resources
		if isClusterDeployment && oc != nil {
			// First remove old URL from OAuthClient
			if resources.RemoveRedirectURIFromOauthClient(oc, hawtio.Status.URL) {
				err := r.client.Update(ctx, oc)
				if err != nil {
					reqLogger.Error(err, "Failed to reconcile OAuth client")
					return reconcile.Result{}, err
				}
			}
			// Add route URL to OAuthClient authorized redirect URIs
			if ok, _ := resources.OauthClientContainsRedirectURI(oc, ingressRouteURL); !ok {
				oc.RedirectURIs = append(oc.RedirectURIs, ingressRouteURL)
				err := r.client.Update(ctx, oc)
				if err != nil {
					reqLogger.Error(err, "Failed to reconcile OAuth client")
					return reconcile.Result{}, err
				}
			}
		}

		if isNamespaceDeployment && oc != nil {
			// Clean-up OAuth client if any. This happens when the deployment type is changed
			// from "cluster" to "namespace".
			if resources.RemoveRedirectURIFromOauthClient(oc, ingressRouteURL) {
				err := r.client.Update(ctx, oc)
				if err != nil {
					reqLogger.Error(err, "Failed to reconcile OAuth client")
					return reconcile.Result{}, err
				}
			}
		}
	}

	reqLogger.V(util.DebugLogLevel).Info("Updating Hawtio custom resource")
	err = r.client.Status().Patch(ctx, hawtioCopy, client.MergeFrom(hawtio))
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to patch status: %v", err)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileHawtio) reconcileConfigMap(hawtio *hawtiov2.Hawtio) (bool, error) {
	reqLogger := log.WithName(fmt.Sprintf("%s-reconcileConfigMap", hawtio.Name))
	configMap, err := resources.NewConfigMap(hawtio, r.apiSpec, reqLogger)
	if err != nil {
		return false, err
	}
	return r.reconcileResources(hawtio, []client.Object{configMap}, []client.ObjectList{&corev1.ConfigMapList{}})
}

func (r *ReconcileHawtio) reconcileDeployment(hawtio *hawtiov2.Hawtio, deploymentConfig DeploymentConfiguration) (bool, error) {
	reqLogger := log.WithName(fmt.Sprintf("%s-reconcileDeployment", hawtio.Name))
	clientCertSecretVersion := ""
	if deploymentConfig.clientCertSecret != nil {
		clientCertSecretVersion = deploymentConfig.clientCertSecret.GetResourceVersion()
	}

	var deployedResources []client.Object
	var resourceListTypes []client.ObjectList

	deployment, err := resources.NewDeployment(
		hawtio, r.apiSpec, deploymentConfig.openShiftConsoleURL,
		deploymentConfig.configMap.GetResourceVersion(), clientCertSecretVersion,
		r.BuildVariables, reqLogger)
	if err != nil {
		return false, err
	}

	deployedResources = append(deployedResources, deployment)
	resourceListTypes = append(resourceListTypes, &appsv1.DeploymentList{})

	service := resources.NewService(hawtio, r.apiSpec, reqLogger)
	deployedResources = append(deployedResources, service)
	resourceListTypes = append(resourceListTypes, &corev1.ServiceList{})

	if r.apiSpec.Routes {
		route := oresources.NewRoute(hawtio, deploymentConfig.tlsRouteSecret, deploymentConfig.caCertRouteSecret, reqLogger)
		deployedResources = append(deployedResources, route)
		resourceListTypes = append(resourceListTypes, &routev1.RouteList{})
	} else {
		ingress := kresources.NewIngress(hawtio, r.apiSpec, deploymentConfig.servingCertSecret, reqLogger)
		deployedResources = append(deployedResources, ingress)
		resourceListTypes = append(resourceListTypes, &networkingv1.IngressList{})
	}

	var serviceAccount *corev1.ServiceAccount
	if hawtio.Spec.Type == hawtiov2.NamespaceHawtioDeploymentType {
		reqLogger.V(util.DebugLogLevel).Info("Adding OAuth client as service account for Namespace mode")

		// Add service account as OAuth client
		serviceAccount, err = resources.NewServiceAccountAsOauthClient(hawtio.Name, hawtio.Spec.ExternalRoutes)
		if err != nil {
			return false, fmt.Errorf("error UpdateResources : %s", err)
		}
		deployedResources = append(deployedResources, serviceAccount)
		resourceListTypes = append(resourceListTypes, &corev1.ServiceAccountList{})
	}

	return r.reconcileResources(hawtio, deployedResources, resourceListTypes)
}

func (r *ReconcileHawtio) reconcileResources(hawtio *hawtiov2.Hawtio,
	requestedResources []client.Object, listObjects []client.ObjectList) (bool, error) {
	reqLogger := log.WithName(fmt.Sprintf("%s-reconcileResources", hawtio.Name))

	for _, res := range requestedResources {
		if res == nil || reflect.ValueOf(res).IsNil() {
			continue
		}
		res.SetNamespace(hawtio.Namespace)
	}

	deployed, err := getDeployedResources(hawtio, r.client, listObjects)
	if err != nil {
		return false, err
	}

	requested := compare.NewMapBuilder().Add(requestedResources...).ResourceMap()

	var hasUpdates bool
	writer := write.New(r.client).WithOwnerController(hawtio, r.scheme)
	comparator := getComparator()
	deltas := comparator.Compare(deployed, requested)
	for resourceType, delta := range deltas {
		reqLogger.V(util.DebugLogLevel).Info(fmt.Sprintf("instances of %s has changes %t", resourceType, delta.HasChanges()))
		if !delta.HasChanges() {
			continue
		}

		reqLogger.Info("", "instances of ", resourceType, "Will create ", len(delta.Added), "update ", len(delta.Updated), "and delete", len(delta.Removed))

		added, err := writer.AddResources(delta.Added)
		if err != nil {
			return false, fmt.Errorf("error AddResources: %s", err)
		}
		updated, err := writer.UpdateResources(deployed[resourceType], delta.Updated)
		if err != nil {
			return false, fmt.Errorf("error UpdateResources : %s", err)
		}
		removed, err := writer.RemoveResources(delta.Removed)
		if err != nil {
			return false, fmt.Errorf("error RemoveResources: %s", err)
		}
		hasUpdates = hasUpdates || added || updated || removed
	}
	return hasUpdates, nil
}

func getComparator() compare.MapComparator {
	resourceComparator := compare.DefaultComparator()

	configMapType := reflect.TypeOf(corev1.ConfigMap{})
	resourceComparator.SetComparator(configMapType, func(deployed client.Object, requested client.Object) bool {
		configMap1 := deployed.(*corev1.ConfigMap)
		configMap2 := requested.(*corev1.ConfigMap)
		var pairs [][2]interface{}
		pairs = append(pairs, [2]interface{}{configMap1.Name, configMap2.Name})
		pairs = append(pairs, [2]interface{}{configMap1.Namespace, configMap2.Namespace})
		pairs = append(pairs, [2]interface{}{configMap1.Labels, configMap2.Labels})
		pairs = append(pairs, [2]interface{}{configMap1.Annotations, configMap2.Annotations})
		pairs = append(pairs, [2]interface{}{configMap1.Data, configMap2.Data})
		pairs = append(pairs, [2]interface{}{configMap1.BinaryData, configMap2.BinaryData})
		equal := compare.EqualPairs(pairs)
		if !equal {
			log.Info("Resources are not equal", "deployed", deployed, "requested", requested)
		}
		return equal
	})

	// Not included by default in comparator
	ingressType := reflect.TypeOf(networkingv1.Ingress{})
	resourceComparator.SetComparator(ingressType, func(deployed client.Object, requested client.Object) bool {
		ingress1 := deployed.(*networkingv1.Ingress)
		ingress2 := requested.(*networkingv1.Ingress)

		ingress1 = ingress1.DeepCopy()

		//Removed generated fields from deployed version, that are not specified in requested item
		emptyString := ""
		if (ingress2.Spec.IngressClassName == &emptyString) {
			ingress1.Spec.IngressClassName = &emptyString
		}

		var pairs [][2]interface{}
		pairs = append(pairs, [2]interface{}{ingress1.Name, ingress2.Name})
		pairs = append(pairs, [2]interface{}{ingress1.Namespace, ingress2.Namespace})
		pairs = append(pairs, [2]interface{}{ingress1.Labels, ingress2.Labels})
		pairs = append(pairs, [2]interface{}{ingress1.Annotations, ingress2.Annotations})
		pairs = append(pairs, [2]interface{}{ingress1.Spec, ingress2.Spec})

		equal := compare.EqualPairs(pairs)
		if !equal {
			log.Info("Resources are not equal", "deployed", deployed, "requested", requested)
		}
		return equal
	})

	return compare.MapComparator{Comparator: resourceComparator}
}

func getDeployedResources(hawtio *hawtiov2.Hawtio, client client.Client, listObjects []client.ObjectList) (map[reflect.Type][]client.Object, error) {
	reader := read.New(client).WithNamespace(hawtio.Namespace).WithOwnerObject(hawtio)
	resourceMap, err := reader.ListAll(listObjects...)
	if err != nil {
		log.Error(err, "Failed to list deployed objects")
		return nil, err
	}

	return resourceMap, nil
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
