package hawtio

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Masterminds/semver"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	"github.com/hawtio/hawtio-operator/pkg/openshift"
	osutil "github.com/hawtio/hawtio-operator/pkg/openshift/util"
	"github.com/hawtio/hawtio-operator/pkg/util"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1 "github.com/openshift/api/apps/v1"
	imagev1 "github.com/openshift/api/image/v1"
	oauthv1 "github.com/openshift/api/oauth/v1"
	routev1 "github.com/openshift/api/route/v1"
	templatev1 "github.com/openshift/api/template/v1"

	configv1client "github.com/openshift/client-go/config/clientset/versioned"
)

var log = logf.Log.WithName("controller_hawtio")

const (
	hawtioFinalizer    = "finalizer.hawtio.hawt.io"
	hawtioTemplatePath = "templates/deployment.yaml"

	configVersionAnnotation = "hawtio.hawt.io/configversion"
	hawtioVersionAnnotation = "hawtio.hawt.io/hawtioversion"
	hawtioTypeAnnotation    = "hawtio.hawt.io/hawtioType"
	hostGeneratedAnnotation = "openshift.io/host.generated"

	hawtioTypeEnvVar        = "HAWTIO_ONLINE_MODE"
	hawtioOAuthClientEnvVar = "HAWTIO_OAUTH_CLIENT_ID"

	oauthClientName                           = "hawtio"
	serviceSigningSecretVolumeMountName       = "hawtio-online-tls-serving"
	serviceSigningSecretVolumeMountPathPre170 = "/etc/tls/private"
	serviceSigningSecretVolumeMountPath       = "/etc/tls/private/serving"
	clientCertificateSecretVolumeMountPath    = "/etc/tls/private/proxying"
)

// Add creates a new Hawtio Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	err := appsv1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}
	err = imagev1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}
	err = oauthv1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}
	err = routev1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}

	r := &ReconcileHawtio{
		client: mgr.GetClient(),
		config: mgr.GetConfig(),
		scheme: mgr.GetScheme(),
	}

	processor, err := openshift.NewTemplateProcessor(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.template = processor

	deployment, err := openshift.NewDeploymentClient(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.deployment = deployment

	oauthclient, err := openshift.NewOAuthClientClient(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.oauthclient = oauthclient

	configclient, err := configv1client.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.configclient = configclient

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
	err = c.Watch(&source.Kind{Type: &appsv1.DeploymentConfig{}}, &handler.EnqueueRequestForOwner{
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
	// that reads objects from the cache and writes to the apiserver
	client       client.Client
	config       *rest.Config
	scheme       *runtime.Scheme
	template     *openshift.TemplateProcessor
	deployment   *openshift.DeploymentClient
	oauthclient  *openshift.OAuthClientClient
	configclient *configv1client.Clientset
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
			return reconcile.Result{}, fmt.Errorf("Deletion failed: %v", err)
		}
		return reconcile.Result{}, nil
	}

	ok, err := util.HasFinalizer(instance, hawtioFinalizer)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("Failed to read finalizer: %v", err)
	}
	if !ok {
		err = util.AddFinalizer(instance, hawtioFinalizer)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("Failed to set finalizer: %v", err)
		}
		err = r.client.Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("Failed to update finalizer: %v", err)
		}
	}

	// Invariant checks
	isClusterDeployment := strings.EqualFold(instance.Spec.Type, hawtiov1alpha1.ClusterHawtioDeploymentType)
	isNamespaceDeployment := strings.EqualFold(instance.Spec.Type, hawtiov1alpha1.NamespaceHawtioDeploymentType)

	if !isNamespaceDeployment && !isClusterDeployment {
		err := fmt.Errorf("Unsupported type: %s", instance.Spec.Type)
		return reconcile.Result{}, err
	}

	// Install phase

	exts, err := r.processTemplate(instance, request)
	if err != nil {
		reqLogger.Error(err, "Error while processing template", "template", hawtioTemplatePath)
		return reconcile.Result{}, err
	}

	objs, err := getRuntimeObjects(exts)
	if err != nil {
		reqLogger.Error(err, "Error while retrieving runtime objects")
		return reconcile.Result{}, err
	}

	if isNamespaceDeployment {
		// Add service account as OAuth client
		sa, err := newServiceAccountAsOauthClient(request.Name)
		if err != nil {
			reqLogger.Error(err, "Error while creating OAuth client")
			return reconcile.Result{}, err
		}
		objs = append(objs, sa)
	}

	// Check OpenShift version
	var openShiftSemVer *semver.Version
	clusterVersion, err := r.configclient.ConfigV1().ClusterVersions().Get("version", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Let's default to OpenShift 3 as ClusterVersion API was introduced in OpenShift 4
			openShiftSemVer, _ = semver.NewVersion("3")
		} else {
			reqLogger.Error(err, "Error parsing version constraint")
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
	constraint, _ := semver.NewConstraint(">= 4")
	isOpenShift4 := constraint.Check(openShiftSemVer)

	// Check console version
	consoleVersion := instance.Spec.Version
	if len(consoleVersion) == 0 {
		consoleVersion = "latest"
	}
	var isConsoleVersion170orHigher bool
	if consoleVersion != "latest" {
		semVer, err := semver.NewVersion(consoleVersion)
		if err != nil {
			reqLogger.Error(err, "Error parsing console semantic version", "version", consoleVersion)
			return reconcile.Result{}, err
		}
		constraint, _ := semver.NewConstraint(">= 1.7.0")
		if constraint.Check(semVer) {
			isConsoleVersion170orHigher = true
		}
	} else {
		isConsoleVersion170orHigher = true
	}

	deploymentConfig := osutil.GetDeploymentConfig(objs)
	container := deploymentConfig.Spec.Template.Spec.Containers[0]

	if isOpenShift4 {
		// Activate console backend gateway
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "HAWTIO_ONLINE_GATEWAY",
			Value: "true",
		})
	}

	// Adjust service signing secret volume mount path
	var volumeMountPath string
	if isConsoleVersion170orHigher {
		volumeMountPath = serviceSigningSecretVolumeMountPath
	} else {
		volumeMountPath = serviceSigningSecretVolumeMountPathPre170
	}
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		MountPath: volumeMountPath,
		Name:      serviceSigningSecretVolumeMountName,
	})

	if isOpenShift4 {
		// Check whether client certificate secret exists
		clientCertificateSecret := &corev1.Secret{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Namespace: request.Namespace, Name: request.Name + "-tls-proxying"}, clientCertificateSecret)
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

		// Mount client certificate secret
		volume := corev1.Volume{
			Name: instance.Name + "-tls-proxying",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: instance.Name + "-tls-proxying",
				},
			},
		}

		deploymentConfig.Spec.Template.Spec.Volumes = append(deploymentConfig.Spec.Template.Spec.Volumes, volume)

		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			MountPath: clientCertificateSecretVolumeMountPath,
			Name:      instance.Name + "-tls-proxying",
		})
	}

	deploymentConfig.Spec.Template.Spec.Containers[0] = container

	// Create runtime objects
	err = r.createObjects(objs, request.Namespace, instance)
	if err != nil {
		reqLogger.Error(err, "Error creating runtime objects")
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
		oauthclient := newOAuthClient()
		err = r.client.Create(context.TODO(), oauthclient)
		if err != nil && !errors.IsAlreadyExists(err) {
			return reconcile.Result{}, err
		}
	}

	// Update phase

	// Reconcile image stream
	stream := &imagev1.ImageStream{}
	err = r.client.Get(context.TODO(), request.NamespacedName, stream)
	if err != nil && errors.IsNotFound(err) {
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get image stream")
		return reconcile.Result{}, err
	}
	// Add tag to the image stream if missing
	var tag *imagev1.TagReference
	if ok, tag = imageStreamContainsTag(stream, consoleVersion); !ok {
		tag = &imagev1.TagReference{
			Name: consoleVersion,
			From: &corev1.ObjectReference{
				Kind: "DockerImage",
				Name: "docker.io/hawtio/online:" + consoleVersion,
			},
			ImportPolicy: imagev1.TagImportPolicy{
				Scheduled: true,
			},
		}
		stream.Spec.Tags = append(stream.Spec.Tags, *tag)
		err := r.client.Update(context.TODO(), stream)
		if err != nil {
			reqLogger.Error(err, "Failed to reconcile to image stream")
			return reconcile.Result{}, err
		}
	}
	// Update CR status image field from image stream
	if instance.Status.Image != tag.From.Name {
		instance.Status.Image = tag.From.Name
		err := r.client.Status().Update(context.TODO(), instance)
		if err != nil {
			reqLogger.Error(err, "Failed to reconcile from image stream")
			return reconcile.Result{}, err
		}
	}

	config := &corev1.ConfigMap{}
	err = r.client.Get(context.TODO(), request.NamespacedName, config)
	if err != nil && errors.IsNotFound(err) {
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get config map")
		return reconcile.Result{}, err
	}

	// Reconcile deployment
	deployment := &appsv1.DeploymentConfig{}
	err = r.client.Get(context.TODO(), request.NamespacedName, deployment)
	if err != nil && errors.IsNotFound(err) {
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get deployment")
		return reconcile.Result{}, err
	}
	updateDeployment := false

	// Reconcile image
	if trigger := fmt.Sprintf("%s:%s", instance.Name, consoleVersion); deployment.Spec.Triggers[0].ImageChangeParams.From.Name != trigger {
		deployment.Spec.Triggers[0].ImageChangeParams.From.Name = trigger
		updateDeployment = true
	}

	// Reconcile service signing secret volume mount path
	container = deployment.Spec.Template.Spec.Containers[0]
	volumeMount, _ := util.GetVolumeMount(container, serviceSigningSecretVolumeMountName)
	if isConsoleVersion170orHigher && volumeMount.MountPath != serviceSigningSecretVolumeMountPath {
		volumeMount.MountPath = serviceSigningSecretVolumeMountPath
		updateDeployment = true
	} else if !isConsoleVersion170orHigher && volumeMount.MountPath != serviceSigningSecretVolumeMountPathPre170 {
		volumeMount.MountPath = serviceSigningSecretVolumeMountPathPre170
		updateDeployment = true
	}

	// Reconcile replicas
	if annotations := deployment.GetAnnotations(); annotations != nil && annotations[hawtioVersionAnnotation] == instance.GetResourceVersion() {
		if replicas := deployment.Spec.Replicas; instance.Spec.Replicas != replicas {
			instance.Spec.Replicas = replicas
			err := r.client.Update(context.TODO(), instance)
			if err != nil {
				reqLogger.Error(err, "Failed to reconcile from deployment")
				return reconcile.Result{}, err
			}
		}
	} else {
		if replicas := instance.Spec.Replicas; deployment.Spec.Replicas != replicas {
			deployment.Annotations[hawtioVersionAnnotation] = instance.GetResourceVersion()
			deployment.Spec.Replicas = replicas
			updateDeployment = true
		}
	}

	// Reconcile environment variables based on deployment type
	envVar, _ := util.GetEnvVarByName(container.Env, hawtioTypeEnvVar)
	if envVar == nil {
		err := fmt.Errorf("Environment variable not found: %s", hawtioTypeEnvVar)
		return reconcile.Result{}, err
	}
	if isClusterDeployment && envVar.Value != strings.ToLower(hawtiov1alpha1.ClusterHawtioDeploymentType) {
		envVar.Value = strings.ToLower(hawtiov1alpha1.ClusterHawtioDeploymentType)
		updateDeployment = true
	}
	if isNamespaceDeployment && envVar.Value != strings.ToLower(hawtiov1alpha1.NamespaceHawtioDeploymentType) {
		envVar.Value = strings.ToLower(hawtiov1alpha1.NamespaceHawtioDeploymentType)
		updateDeployment = true
	}

	envVar, _ = util.GetEnvVarByName(container.Env, hawtioOAuthClientEnvVar)
	if envVar == nil {
		err := fmt.Errorf("Environment variable not found: %s", hawtioOAuthClientEnvVar)
		return reconcile.Result{}, err
	}
	if isClusterDeployment && envVar.Value != oauthClientName {
		envVar.Value = oauthClientName
		updateDeployment = true
	}
	if isNamespaceDeployment && envVar.Value != instance.Name {
		envVar.Value = instance.Name
		updateDeployment = true
	}

	requestDeployment := false
	if configVersion := config.GetResourceVersion(); deployment.Annotations[configVersionAnnotation] != configVersion {
		if len(deployment.Annotations[configVersionAnnotation]) > 0 {
			requestDeployment = true
		}
		deployment.Annotations[configVersionAnnotation] = configVersion
		updateDeployment = true
	}

	if updateDeployment {
		err := r.client.Update(context.TODO(), deployment)
		if err != nil {
			reqLogger.Error(err, "Failed to reconcile to deployment")
			return reconcile.Result{}, err
		}
	}
	if requestDeployment {
		rollout := &appsv1.DeploymentRequest{
			TypeMeta: metav1.TypeMeta{
				Kind:       "DeploymentRequest",
				APIVersion: "apps.openshift.io/v1",
			},
			Name:   request.NamespacedName.Name,
			Latest: true,
			Force:  true,
		}
		_, err := r.deployment.Deploy(rollout, request.Namespace)
		if err != nil {
			reqLogger.Error(err, "Failed to rollout deployment")
			return reconcile.Result{}, err
		}
	}

	// Reconcile route
	route := &routev1.Route{}
	err = r.client.Get(context.TODO(), request.NamespacedName, route)
	if err != nil && errors.IsNotFound(err) {
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get route")
		return reconcile.Result{}, err
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
	if hostName := instance.Spec.RouteHostName; len(hostName) == 0 && !strings.EqualFold(route.Annotations[hostGeneratedAnnotation], "true") {
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

	// Reconcile OAuth client
	// Do not use the default client whose cached informers require
	// permission to list cluster wide oauth clients
	// err = r.client.Get(context.TODO(), types.NamespacedName{Name: oauthClientName}, oc)
	oc, err := r.oauthclient.Get(oauthClientName)
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

	// TODO: OAuth client reconciliation triggered by rollout deployment should ideally
	// wait until the deployment is successful before deleting resources.
	if url := osutil.GetRouteURL(route); instance.Status.URL != url {
		if isClusterDeployment {
			// First remove old URL from OAuthClient
			if removeRedirectURIFromOauthClient(oc, instance.Status.URL) {
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
		uri := osutil.GetRouteURL(route)
		if ok, _ := oauthClientContainsRedirectURI(oc, uri); !ok {
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
		uri := osutil.GetRouteURL(route)
		if removeRedirectURIFromOauthClient(oc, uri) {
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
	// and report back the type and version into the owned deployment
	if annotations := deployment.GetAnnotations(); annotations != nil && (!strings.EqualFold(annotations[hawtioTypeAnnotation], instance.Spec.Type) || annotations[hawtioVersionAnnotation] != instance.GetResourceVersion()) {
		deployment.Annotations[hawtioTypeAnnotation] = instance.Spec.Type
		deployment.Annotations[hawtioVersionAnnotation] = instance.GetResourceVersion()
		err := r.client.Update(context.TODO(), deployment)
		if err != nil {
			reqLogger.Error(err, "Failed to refresh deployment owner annotations")
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileHawtio) processTemplate(cr *hawtiov1alpha1.Hawtio, request reconcile.Request) ([]runtime.RawExtension, error) {
	res, err := osutil.LoadKubernetesResourceFromFile(hawtioTemplatePath)
	if err != nil {
		return nil, fmt.Errorf("Error reading template: %s", err)
	}

	parameters := make(map[string]string)
	parameters["APPLICATION_NAME"] = cr.Name
	parameters["DEPLOYMENT_TYPE"] = cr.Spec.Type

	if version := cr.Spec.Version; len(version) > 0 {
		parameters["VERSION"] = version
	}

	if strings.EqualFold(cr.Spec.Type, hawtiov1alpha1.ClusterHawtioDeploymentType) {
		parameters["OAUTH_CLIENT"] = oauthClientName
	} else if strings.EqualFold(cr.Spec.Type, hawtiov1alpha1.NamespaceHawtioDeploymentType) {
		parameters["OAUTH_CLIENT"] = cr.Name
	}

	if replicas := cr.Spec.Replicas; replicas > 0 {
		parameters["REPLICAS"] = fmt.Sprint(replicas)
	}
	if route := cr.Spec.RouteHostName; len(route) > 0 {
		parameters["ROUTE_HOSTNAME"] = route
	}

	return r.template.Process(res.(*templatev1.Template), request.Namespace, parameters)
}

func (r *ReconcileHawtio) createObjects(objects []runtime.Object, ns string, cr *hawtiov1alpha1.Hawtio) error {
	for _, o := range objects {
		uo, err := osutil.UnstructuredFromRuntimeObject(o)
		if err != nil {
			return fmt.Errorf("failed to transform object: %v", err)
		}

		uo.SetNamespace(ns)
		err = controllerutil.SetControllerReference(cr, uo, r.scheme)
		if err != nil {
			return fmt.Errorf("failed to set owner in object: %v", err)
		}

		annotations := uo.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{
				hawtioVersionAnnotation: cr.GetResourceVersion(),
			}
		} else {
			annotations[hawtioVersionAnnotation] = cr.GetResourceVersion()
		}
		uo.SetAnnotations(annotations)

		err = r.client.Create(context.TODO(), uo.DeepCopyObject())
		if err != nil {
			if errors.IsAlreadyExists(err) {
				// FIXME: apply CR spec to existing resources
				continue
			}
			return fmt.Errorf("failed to create object: %v", err)
		}
	}

	return nil
}

func (r *ReconcileHawtio) deletion(cr *hawtiov1alpha1.Hawtio) error {
	ok, err := util.HasFinalizer(cr, "foregroundDeletion")
	if err != nil {
		return err
	}
	if ok {
		return nil
	}

	if strings.EqualFold(cr.Spec.Type, hawtiov1alpha1.ClusterHawtioDeploymentType) {
		// Remove URI for OAuth client
		oc := &oauthv1.OAuthClient{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: oauthClientName}, oc)
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("Failed to get OAuth client: %v", err)
		}
		updated := removeRedirectURIFromOauthClient(oc, cr.Status.URL)
		if updated {
			err := r.client.Update(context.TODO(), oc)
			if err != nil {
				return fmt.Errorf("Failed to remove redirect URI from OAuth client: %v", err)
			}
		}
	}

	_, err = util.RemoveFinalizer(cr, hawtioFinalizer)
	if err != nil {
		return err
	}

	err = r.client.Update(context.TODO(), cr)
	if err != nil {
		return fmt.Errorf("Failed to remove finalizer: %v", err)
	}

	return nil
}

func getRuntimeObjects(exts []runtime.RawExtension) ([]runtime.Object, error) {
	objects := make([]runtime.Object, 0)

	for _, ext := range exts {
		res, err := osutil.LoadKubernetesResource(ext.Raw)
		if err != nil {
			return nil, err
		}
		objects = append(objects, res)
	}

	return objects, nil
}
