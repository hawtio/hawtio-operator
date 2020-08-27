package hawtio

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Masterminds/semver"

	"github.com/RHsyseng/operator-utils/pkg/resource"
	"github.com/RHsyseng/operator-utils/pkg/resource/compare"
	"github.com/RHsyseng/operator-utils/pkg/resource/read"
	"github.com/RHsyseng/operator-utils/pkg/resource/write"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	consolev1 "github.com/openshift/api/console/v1"
	oauthv1 "github.com/openshift/api/oauth/v1"
	routev1 "github.com/openshift/api/route/v1"
	configclient "github.com/openshift/client-go/config/clientset/versioned"
	oauthclient "github.com/openshift/client-go/oauth/clientset/versioned"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	"github.com/hawtio/hawtio-operator/pkg/kubernetes"
	"github.com/hawtio/hawtio-operator/pkg/openshift"
	"github.com/hawtio/hawtio-operator/pkg/resources"
)

var log = logf.Log.WithName("controller_hawtio")

const (
	hawtioFinalizer = "finalizer.hawtio.hawt.io"

	configVersionAnnotation     = "hawtio.hawt.io/configversion"
	deploymentRolloutAnnotation = "hawtio.hawt.io/restartedAt"
	hawtioVersionAnnotation     = "hawtio.hawt.io/hawtioversion"
	hostGeneratedAnnotation     = "openshift.io/host.generated"

	clientCertificateSecretVolumeName         = "hawtio-online-tls-proxying"
	serviceSigningSecretVolumeName            = "hawtio-online-tls-serving"
	serviceSigningSecretVolumeMountPathLegacy = "/etc/tls/private"
	serviceSigningSecretVolumeMountPath       = "/etc/tls/private/serving"
	clientCertificateSecretVolumeMountPath    = "/etc/tls/private/proxying"
)

// Go build-time variables
var LegacyServingCertificateMountVersion string

// Add creates a new Hawtio Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
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
	err = apiextensionsv1beta1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}

	r := &ReconcileHawtio{
		client: mgr.GetClient(),
		config: mgr.GetConfig(),
		scheme: mgr.GetScheme(),
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

	coreClient, err := corev1client.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.coreClient = coreClient

	if err := openshift.ConsoleYAMLSampleExists(); err == nil {
		openshift.CreateConsoleYAMLSamples(mgr.GetClient())
	}

	return add(mgr, r)
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("hawtio-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Hawtio
	err = c.Watch(&source.Kind{Type: &hawtiov1alpha1.Hawtio{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Ignore updates to CR status in which case metadata.Generation does not change
			return e.MetaOld.GetGeneration() != e.MetaNew.GetGeneration()
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Evaluates to false if the object has been confirmed deleted
			return !e.DeleteStateUnknown
		},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resources and requeue the owner Hawtio
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hawtiov1alpha1.Hawtio{},
	})
	if err != nil {
		return err
	}
	err = c.Watch(&source.Kind{Type: &routev1.Route{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hawtiov1alpha1.Hawtio{},
	})
	if err != nil {
		return err
	}
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hawtiov1alpha1.Hawtio{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileHawtio{}

// ReconcileHawtio reconciles a Hawtio object
type ReconcileHawtio struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the API server
	client       client.Client
	config       *rest.Config
	scheme       *runtime.Scheme
	coreClient   *corev1client.CoreV1Client
	oauthClient  oauthclient.Interface
	configClient configclient.Interface
}

// Reconcile reads that state of the cluster for a Hawtio object and makes changes based on the state read
// and what is in the Hawtio.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileHawtio) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Hawtio")

	// Fetch the Hawtio instance
	instance := &hawtiov1alpha1.Hawtio{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Delete phase

	if instance.GetDeletionTimestamp() != nil {
		err = r.deletion(instance)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("deletion failed: %v", err)
		}
		return reconcile.Result{}, nil
	}

	// TODO: only add the finalizer for cluster mode
	ok, err := kubernetes.HasFinalizer(instance, hawtioFinalizer)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to read finalizer: %v", err)
	}
	if !ok {
		err = kubernetes.AddFinalizer(instance, hawtioFinalizer)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to set finalizer: %v", err)
		}
		err = r.client.Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update finalizer: %v", err)
		}
	}

	// Init phase

	if len(instance.Spec.Type) == 0 {
		instance.Spec.Type = hawtiov1alpha1.ClusterHawtioDeploymentType
		err = r.client.Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update type: %v", err)
		}
		return reconcile.Result{}, nil
	}

	// Invariant checks
	isClusterDeployment := strings.EqualFold(instance.Spec.Type, hawtiov1alpha1.ClusterHawtioDeploymentType)
	isNamespaceDeployment := strings.EqualFold(instance.Spec.Type, hawtiov1alpha1.NamespaceHawtioDeploymentType)

	if !isNamespaceDeployment && !isClusterDeployment {
		err := fmt.Errorf("unsupported type: %s", instance.Spec.Type)
		if instance.Status.Phase != hawtiov1alpha1.HawtioPhaseFailed {
			instance.Status.Phase = hawtiov1alpha1.HawtioPhaseFailed
			err = r.client.Status().Update(context.TODO(), instance)
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to update phase: %v", err)
			}
		}
		return reconcile.Result{}, err
	}

	if len(instance.Status.Phase) == 0 || instance.Status.Phase == hawtiov1alpha1.HawtioPhaseFailed {
		instance.Status.Phase = hawtiov1alpha1.HawtioPhaseInitialized
		err = r.client.Status().Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update phase: %v", err)
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Check OpenShift version
	var openShiftSemVer *semver.Version
	clusterVersion, err := r.configClient.ConfigV1().ClusterVersions().Get("version", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Let's default to OpenShift 3 as ClusterVersion API was introduced in OpenShift 4
			openShiftSemVer, _ = semver.NewVersion("3")
		} else {
			reqLogger.Error(err, "Error reading cluster version")
			return reconcile.Result{}, err
		}
	} else {
		// Let's take the latest version from the history
		v := clusterVersion.Status.History[0].Version
		openShiftSemVer, err = semver.NewVersion(v)
		if err != nil {
			reqLogger.Error(err, "Error parsing cluster semantic version", "version", v)
			return reconcile.Result{}, err
		}
	}
	constraint4, _ := semver.NewConstraint(">= 4-0")
	isOpenShift4 := constraint4.Check(openShiftSemVer)
	constraint43, _ := semver.NewConstraint(">= 4.3")
	isOpenShift43Plus := constraint43.Check(openShiftSemVer)

	// Check Hawtio console version
	consoleVersion := instance.Spec.Version
	if len(consoleVersion) == 0 {
		consoleVersion = "latest"
	}

	var openShiftConsoleUrl string
	if isOpenShift4 {
		// Retrieve OpenShift Web console public URL
		cm, err := r.coreClient.ConfigMaps("openshift-config-managed").Get("console-public", metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) && !errors.IsForbidden(err) {
				reqLogger.Error(err, "Error getting OpenShift managed configuration")
				return reconcile.Result{}, err
			}
		} else {
			openShiftConsoleUrl = cm.Data["consoleURL"]
		}
	}

	// Adjust service signing secret volume mount path
	serviceSigningCertificateVolumeMountPath, err := getServingCertificateMountPathFor(consoleVersion)
	if err != nil {
		reqLogger.Error(err, "Error getting service signing certificate mount path", "version", consoleVersion)
		return reconcile.Result{}, err
	}
	if isOpenShift4 {
		// Check whether client certificate secret exists
		_, err := r.coreClient.
			Secrets(request.Namespace).
			Get(request.Name+"-tls-proxying", metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				reqLogger.Info("Client certificate secret must be created", "secret", request.Name+"-tls-proxying")
				// Let's poll for the client certificate secret to be created
				return reconcile.Result{
					Requeue:      true,
					RequeueAfter: 5 * time.Second,
				}, nil
			} else {
				return reconcile.Result{}, err
			}
		}
	}

	configMap := &corev1.ConfigMap{}
	err = r.client.Get(context.TODO(), request.NamespacedName, configMap)
	if err != nil && errors.IsNotFound(err) {
		configMap, err = resources.NewConfigMapForCR(instance)
		if err != nil {
			reqLogger.Error(err, "Failed to generate config map")
			return reconcile.Result{}, err
		}
		err = r.client.Create(context.TODO(), configMap)
		if err != nil {
			reqLogger.Error(err, "Failed to create config map")
			return reconcile.Result{}, err
		}
	} else if err != nil {
		reqLogger.Error(err, "Failed to get config map")
		return reconcile.Result{}, err
	}

	_, err = r.reconcileResources(instance, request, r.client, r.scheme, isOpenShift4, openShiftSemVer.String(), openShiftConsoleUrl, serviceSigningCertificateVolumeMountPath, configMap)
	if err != nil {
		reqLogger.Error(err, "Error reconciling resources")
		return reconcile.Result{}, err
	}

	route := &routev1.Route{}
	err = r.client.Get(context.TODO(), request.NamespacedName, route)
	if err != nil && errors.IsNotFound(err) {
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get route")
		return reconcile.Result{}, err
	}

	if isClusterDeployment {
		// Clean-up service account as OAuth client if any.
		// This happens when the deployment type is changed
		// from "namespace" to "cluster".
		sa := &corev1.ServiceAccount{}
		err = r.client.Get(context.TODO(), request.NamespacedName, sa)
		if err != nil {
			if !errors.IsNotFound(err) {
				reqLogger.Error(err, "Failed to get service account")
				return reconcile.Result{}, err
			}
		} else {
			err = r.client.Delete(context.TODO(), sa)
			if err != nil {
				reqLogger.Error(err, "Failed to delete service account")
				return reconcile.Result{}, err
			}
		}
		// Add OAuth client
		oauthClient := resources.NewOAuthClient(resources.OAuthClientName)
		err = r.client.Create(context.TODO(), oauthClient)
		if err != nil && !errors.IsAlreadyExists(err) {
			return reconcile.Result{}, err
		}
	}

	// Read Hawtio configuration
	hawtconfig, err := resources.GetHawtioConfig(configMap)
	if err != nil {
		reqLogger.Error(err, "Failed to get hawtconfig")
		return reconcile.Result{}, err
	}

	// Add link to OpenShift console
	consoleLinkName := request.Name + "-" + request.Namespace
	if isOpenShift4 && instance.Status.Phase == hawtiov1alpha1.HawtioPhaseInitialized {
		consoleLink := &consolev1.ConsoleLink{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: consoleLinkName}, consoleLink)
		if err != nil {
			if !errors.IsNotFound(err) {
				reqLogger.Error(err, "Failed to get console link")
				return reconcile.Result{}, err
			}
		} else {
			err = r.client.Delete(context.TODO(), consoleLink)
			if err != nil {
				reqLogger.Error(err, "Failed to delete console link")
				return reconcile.Result{}, err
			}
		}
		consoleLink = &consolev1.ConsoleLink{}
		if isClusterDeployment {
			consoleLink = openshift.NewApplicationMenuLink(consoleLinkName, route, hawtconfig)
		} else if isOpenShift43Plus {
			consoleLink = openshift.NewNamespaceDashboardLink(consoleLinkName, request.Namespace, route, hawtconfig)
		}
		if consoleLink.Spec.Location != "" {
			err = r.client.Create(context.TODO(), consoleLink)
			if err != nil {
				reqLogger.Error(err, "Failed to create console link", "name", consoleLink.Name)
				return reconcile.Result{}, err
			}
		}
	}

	// Update status
	if instance.Status.Phase != hawtiov1alpha1.HawtioPhaseDeployed {
		instance.Status.Phase = hawtiov1alpha1.HawtioPhaseDeployed
		err = r.client.Status().Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update phase: %v", err)
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Update phase

	// Reconcile deployment
	deployment := &appsv1.Deployment{}
	updateDeployment := false
	err = r.client.Get(context.TODO(), request.NamespacedName, deployment)
	if err != nil && errors.IsNotFound(err) {
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get deployment")
		return reconcile.Result{}, err
	}

	container := deployment.Spec.Template.Spec.Containers[0]

	// Reconcile replicas
	if annotations := deployment.GetAnnotations(); annotations != nil && annotations[hawtioVersionAnnotation] == instance.GetResourceVersion() {
		if replicas := deployment.Spec.Replicas; replicas != nil && instance.Spec.Replicas != *replicas {
			instance.Spec.Replicas = *replicas
			err := r.client.Update(context.TODO(), instance)
			if err != nil {
				reqLogger.Error(err, "Failed to reconcile from deployment")
				return reconcile.Result{}, err
			}
		}
	}

	requestDeployment := false
	if configVersion := configMap.GetResourceVersion(); deployment.Annotations[configVersionAnnotation] != configVersion {
		if len(deployment.Annotations[configVersionAnnotation]) > 0 {
			requestDeployment = true
		}
		deployment.Annotations[configVersionAnnotation] = configVersion
		updateDeployment = true
	}

	if requestDeployment {
		// similar to `kubectl rollout restart`
		if deployment.Spec.Template.ObjectMeta.Annotations == nil {
			deployment.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
		}
		deployment.Spec.Template.ObjectMeta.Annotations[deploymentRolloutAnnotation] = time.Now().Format(time.RFC3339)
		updateDeployment = true
	}

	if updateDeployment {
		err := r.client.Update(context.TODO(), deployment)
		if err != nil {
			reqLogger.Error(err, "Failed to reconcile to deployment")
			return reconcile.Result{}, err
		}
	}

	// Update CR status image field from deployment container image
	if instance.Status.Image != container.Image {
		instance.Status.Image = container.Image
		err := r.client.Status().Update(context.TODO(), instance)
		if err != nil {
			reqLogger.Error(err, "Failed to reconcile image status from deployment")
			return reconcile.Result{}, err
		}
	}

	// Reconcile route host from routeHostName field
	if hostName := instance.Spec.RouteHostName; len(hostName) > 0 && hostName != route.Spec.Host {
		if _, ok := route.Annotations[hostGeneratedAnnotation]; ok {
			delete(route.Annotations, hostGeneratedAnnotation)
		}
		route.Spec.Host = hostName
		err := r.client.Update(context.TODO(), route)
		if err != nil {
			reqLogger.Error(err, "Failed to reconcile route from CR")
			return reconcile.Result{}, err
		}
	}
	if hostName := instance.Spec.RouteHostName; len(hostName) == 0 && !strings.EqualFold(route.Annotations[hostGeneratedAnnotation], "true") && route.Generation > 0 {
		// Emptying route host is ignored so it's not possible to re-generate the host
		// See https://github.com/openshift/origin/pull/9425
		// In that case, let's delete the route
		err := r.client.Delete(context.TODO(), route)
		if err != nil {
			reqLogger.Error(err, "Failed to delete route to auto-generate hostname")
			return reconcile.Result{}, err
		}
		// And requeue to create a new route in the next reconcile loop
		return reconcile.Result{Requeue: true}, nil
	}

	// Reconcile console link in OpenShift console
	if isOpenShift4 {
		consoleLink := &consolev1.ConsoleLink{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: consoleLinkName}, consoleLink)
		if err != nil {
			if errors.IsNotFound(err) {
				// If not found, create a console link
				if isClusterDeployment {
					consoleLink = openshift.NewApplicationMenuLink(consoleLinkName, route, hawtconfig)
				} else if isOpenShift43Plus {
					consoleLink = openshift.NewNamespaceDashboardLink(consoleLinkName, request.Namespace, route, hawtconfig)
				}
				if consoleLink.Spec.Location != "" {
					err = r.client.Create(context.TODO(), consoleLink)
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
				openshift.UpdateApplicationMenuLink(consoleLinkCopy, route, hawtconfig)
			} else if isOpenShift43Plus {
				openshift.UpdateNamespaceDashboardLink(consoleLinkCopy, route, hawtconfig)
			}
			patch := client.MergeFrom(consoleLink)
			err = r.client.Patch(context.TODO(), consoleLinkCopy, patch)
			if err != nil {
				reqLogger.Error(err, "Failed to update console link", "name", consoleLink.Name)
				return reconcile.Result{}, err
			}
		}
	}

	// Reconcile OAuth client
	// Do not use the default client whose cached informers require
	// permission to list cluster wide oauth clients
	// err = r.client.Get(context.TODO(), types.NamespacedName{Name: resources.OAuthClientName}, oc)
	oc, err := r.oauthClient.OauthV1().OAuthClients().Get(resources.OAuthClientName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// OAuth client should not be found for namespace deployment type
			// except when it changes from "cluster" to "namespace"
			if isClusterDeployment {
				return reconcile.Result{Requeue: true}, nil
			}
		} else if !(errors.IsForbidden(err) && isNamespaceDeployment) {
			// We tolerate 403 for namespace deployment as the operator
			// may not have permission to read cluster wide resources
			// like OAuth clients
			reqLogger.Error(err, "Failed to get OAuth client")
			return reconcile.Result{}, err
		}
	}

	// TODO: OAuth client reconciliation triggered by roll-out deployment should ideally
	// wait until the deployment is successful before deleting resources.
	if url := resources.GetRouteURL(route); instance.Status.URL != url {
		if isClusterDeployment {
			// First remove old URL from OAuthClient
			if resources.RemoveRedirectURIFromOauthClient(oc, instance.Status.URL) {
				err := r.client.Update(context.TODO(), oc)
				if err != nil {
					reqLogger.Error(err, "Failed to reconcile OAuth client")
					return reconcile.Result{}, err
				}
			}
		}
		instance.Status.URL = url
		err := r.client.Status().Update(context.TODO(), instance)
		if err != nil {
			reqLogger.Error(err, "Failed to reconcile from route")
			return reconcile.Result{}, err
		}
	}

	if isClusterDeployment {
		// Add route URL to OAuthClient authorized redirect URIs
		uri := resources.GetRouteURL(route)
		if ok, _ := resources.OauthClientContainsRedirectURI(oc, uri); !ok {
			oc.RedirectURIs = append(oc.RedirectURIs, uri)
			err := r.client.Update(context.TODO(), oc)
			if err != nil {
				reqLogger.Error(err, "Failed to reconcile OAuth client")
				return reconcile.Result{}, err
			}
		}
	}
	if isNamespaceDeployment && oc != nil {
		// Clean-up OAuth client if any.
		// This happens when the deployment type is changed
		// from "cluster" to "namespace".
		uri := resources.GetRouteURL(route)
		if resources.RemoveRedirectURIFromOauthClient(oc, uri) {
			err := r.client.Update(context.TODO(), oc)
			if err != nil {
				reqLogger.Error(err, "Failed to reconcile OAuth client")
				return reconcile.Result{}, err
			}
		}
	}
	// Refresh the instance
	instance = &hawtiov1alpha1.Hawtio{}
	err = r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		reqLogger.Error(err, "Failed to refresh CR")
		return reconcile.Result{}, err
	}
	// and report back the version into the owned deployment
	if annotations := deployment.GetAnnotations(); annotations != nil && (annotations[hawtioVersionAnnotation] != instance.GetResourceVersion()) {
		deployment.Annotations[hawtioVersionAnnotation] = instance.GetResourceVersion()
		err := r.client.Update(context.TODO(), deployment)
		if err != nil {
			reqLogger.Error(err, "Failed to refresh deployment owner annotations")
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileHawtio) reconcileResources(cr *hawtiov1alpha1.Hawtio, request reconcile.Request, client client.Client, scheme *runtime.Scheme, isOpenShift4 bool, openshiftVersion string, openshiftURL string, volumePath string, configMap *corev1.ConfigMap) (bool, error) {
	reqLogger := log.WithName(cr.Name)

	isNamespaceDeployment := strings.EqualFold(cr.Spec.Type, hawtiov1alpha1.NamespaceHawtioDeploymentType)

	dep := resources.NewDeploymentForCR(cr, isOpenShift4, openshiftVersion, openshiftURL, volumePath, configMap.GetResourceVersion())
	serviceDefinition := resources.NewServiceDefinitionForCR(cr)
	routeDefinition := resources.NewRouteDefinitionForCR(cr)

	var requestedResources []resource.KubernetesResource
	//requestedResources = append(requestedResources, configMap)
	requestedResources = append(requestedResources, dep)
	requestedResources = append(requestedResources, serviceDefinition)
	requestedResources = append(requestedResources, routeDefinition)

	if isNamespaceDeployment {
		// Add service account as OAuth client
		sa, err := resources.NewServiceAccountAsOauthClient(request.Name)
		if err != nil {
			return false, fmt.Errorf("error UpdateResources : %s", err)
		}
		requestedResources = append(requestedResources, sa)
	}

	for index := range requestedResources {
		requestedResources[index].SetNamespace(cr.Namespace)
	}

	deployed, err := getDeployedResources(cr, client)
	if err != nil {
		return false, err
	}

	requested := compare.NewMapBuilder().Add(requestedResources...).ResourceMap()

	var hasUpdates bool
	writer := write.New(client).WithOwnerController(cr, scheme)
	comparator := getComparator()
	deltas := comparator.Compare(deployed, requested)
	for resourceType, delta := range deltas {
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
	resourceComparator.SetComparator(configMapType, func(deployed resource.KubernetesResource, requested resource.KubernetesResource) bool {
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

	return compare.MapComparator{Comparator: resourceComparator}
}

func getDeployedResources(cr *hawtiov1alpha1.Hawtio, client client.Client) (map[reflect.Type][]resource.KubernetesResource, error) {
	reader := read.New(client).WithNamespace(cr.Namespace).WithOwnerObject(cr)
	resourceMap, err := reader.ListAll(
		&corev1.ConfigMapList{},
		&corev1.ServiceList{},
		&appsv1.DeploymentList{},
		&routev1.RouteList{},
		&corev1.ServiceAccountList{},
	)
	if err != nil {
		log.Error(err, "Failed to list deployed objects. ", err)
		return nil, err
	}

	return resourceMap, nil
}

func (r *ReconcileHawtio) update(cr *hawtiov1alpha1.Hawtio) (reconcile.Result, error) {
	err := r.client.Update(context.TODO(), cr)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{Requeue: true}, nil
}

func (r *ReconcileHawtio) deletion(cr *hawtiov1alpha1.Hawtio) error {
	ok, err := kubernetes.HasFinalizer(cr, "foregroundDeletion")
	if err != nil {
		return err
	}
	if ok {
		return nil
	}

	if strings.EqualFold(cr.Spec.Type, hawtiov1alpha1.ClusterHawtioDeploymentType) {
		// Remove URI from OAuth client
		oc := &oauthv1.OAuthClient{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: resources.OAuthClientName}, oc)
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get OAuth client: %v", err)
		}
		updated := resources.RemoveRedirectURIFromOauthClient(oc, cr.Status.URL)
		if updated {
			err := r.client.Update(context.TODO(), oc)
			if err != nil {
				return fmt.Errorf("failed to remove redirect URI from OAuth client: %v", err)
			}
		}
	}

	// Remove OpenShift console link
	consoleLink := &consolev1.ConsoleLink{
		ObjectMeta: metav1.ObjectMeta{
			Name: cr.ObjectMeta.Name + "-" + cr.ObjectMeta.Namespace,
		},
	}
	err = r.client.Delete(context.TODO(), consoleLink)
	if err != nil && !errors.IsNotFound(err) && !meta.IsNoMatchError(err) {
		return fmt.Errorf("failed to delete console link: %v", err)
	}

	_, err = kubernetes.RemoveFinalizer(cr, hawtioFinalizer)
	if err != nil {
		return err
	}

	err = r.client.Update(context.TODO(), cr)
	if err != nil {
		return fmt.Errorf("failed to remove finalizer: %v", err)
	}

	return nil
}

func getServingCertificateMountPathFor(version string) (string, error) {
	if version != "latest" {
		semVer, err := semver.NewVersion(version)
		if err != nil {
			return "", err
		}
		var constraints *semver.Constraints
		if LegacyServingCertificateMountVersion == "" {
			constraints, err = semver.NewConstraint("< 1.7.0")
			if err != nil {
				return "", err
			}
		} else {
			constraints, err = semver.NewConstraint(LegacyServingCertificateMountVersion)
			if err != nil {
				return "", err
			}
		}
		if constraints.Check(semVer) {
			return serviceSigningSecretVolumeMountPathLegacy, nil
		}
	}
	return serviceSigningSecretVolumeMountPath, nil
}
