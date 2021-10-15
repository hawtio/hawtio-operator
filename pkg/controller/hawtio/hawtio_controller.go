package hawtio

import (
	"context"
	"fmt"
	"k8s.io/api/batch/v1beta1"
	"os"
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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
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

	consolev1 "github.com/openshift/api/console/v1"
	oauthv1 "github.com/openshift/api/oauth/v1"
	routev1 "github.com/openshift/api/route/v1"
	configclient "github.com/openshift/client-go/config/clientset/versioned"
	oauthclient "github.com/openshift/client-go/oauth/clientset/versioned"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	"github.com/hawtio/hawtio-operator/pkg/openshift"
	"github.com/hawtio/hawtio-operator/pkg/resources"
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

	coreClient, err := corev1client.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.coreClient = coreClient

	if err := openshift.ConsoleYAMLSampleExists(); err == nil {
		openshift.CreateConsoleYAMLSamples(context.TODO(), mgr.GetClient(), r.ProductName)
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
	}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldDeployment := e.ObjectOld.(*appsv1.Deployment)
			newDeployment := e.ObjectNew.(*appsv1.Deployment)
			// Ignore updates to the Deployment other than the replicas one,
			// that are used to reconcile the Hawtio replicas.
			return oldDeployment.Status.Replicas != newDeployment.Status.Replicas
		},
	})
	//watch secret
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
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
	util.BuildVariables
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the API server
	client       client.Client
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

	ctx := context.TODO()

	// Fetch the Hawtio instance
	hawtio := &hawtiov1alpha1.Hawtio{}
	err := r.client.Get(ctx, request.NamespacedName, hawtio)
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

	if hawtio.GetDeletionTimestamp() != nil {
		err = r.deletion(ctx, hawtio)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("deletion failed: %v", err)
		}
		return reconcile.Result{}, nil
	}

	// Add a finalizer, that's needed to clean up cluster-wide resources, like ConsoleLink and OAuthClient
	if !controllerutil.ContainsFinalizer(hawtio, hawtioFinalizer) {
		controllerutil.AddFinalizer(hawtio, hawtioFinalizer)
		err = r.client.Update(ctx, hawtio)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update finalizer: %v", err)
		}
	}

	// Init phase

	if len(hawtio.Spec.Type) == 0 {
		hawtio.Spec.Type = hawtiov1alpha1.ClusterHawtioDeploymentType
		err = r.client.Update(ctx, hawtio)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update type: %v", err)
		}
		return reconcile.Result{}, nil
	}

	// Invariant checks
	isClusterDeployment := hawtio.Spec.Type == hawtiov1alpha1.ClusterHawtioDeploymentType
	isNamespaceDeployment := hawtio.Spec.Type == hawtiov1alpha1.NamespaceHawtioDeploymentType

	if !isNamespaceDeployment && !isClusterDeployment {
		err := fmt.Errorf("unsupported type: %s", hawtio.Spec.Type)
		if hawtio.Status.Phase != hawtiov1alpha1.HawtioPhaseFailed {
			previous := hawtio.DeepCopy()
			hawtio.Status.Phase = hawtiov1alpha1.HawtioPhaseFailed
			err = r.client.Status().Patch(ctx, hawtio, client.MergeFrom(previous))
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to update phase: %v", err)
			}
		}
		return reconcile.Result{}, err
	}

	if len(hawtio.Status.Phase) == 0 || hawtio.Status.Phase == hawtiov1alpha1.HawtioPhaseFailed {
		previous := hawtio.DeepCopy()
		hawtio.Status.Phase = hawtiov1alpha1.HawtioPhaseInitialized
		err = r.client.Status().Patch(ctx, hawtio, client.MergeFrom(previous))
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update phase: %v", err)
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Check OpenShift version
	var openShiftSemVer *semver.Version
	clusterVersion, err := r.configClient.ConfigV1().ClusterVersions().Get(ctx, "version", metav1.GetOptions{})
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

	var openShiftConsoleUrl string
	if isOpenShift4 {
		// Retrieve OpenShift Web console public URL
		cm, err := r.coreClient.ConfigMaps("openshift-config-managed").Get(ctx, "console-public", metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) && !errors.IsForbidden(err) {
				reqLogger.Error(err, "Error getting OpenShift managed configuration")
				return reconcile.Result{}, err
			}
		} else {
			openShiftConsoleUrl = cm.Data["consoleURL"]
		}
	}

	if isOpenShift4 {
		cronJob := &v1beta1.CronJob{}
		cronJobName := request.Name + "-certificate-expiry-check"
		cronJobErr := r.client.Get(ctx, client.ObjectKey{Namespace: request.Namespace, Name: cronJobName}, cronJob)

		// Check whether client certificate secret exists
		secretName := request.Name + "-tls-proxying"
		clientCertSecret := corev1.Secret{}
		err := r.client.Get(ctx, client.ObjectKey{Namespace: request.Namespace, Name: secretName}, &clientCertSecret)
		// TODO: Update the client certificate when the Hawtio resource changes
		if err != nil {
			if errors.IsNotFound(err) {
				reqLogger.Info("Client certificate secret not found, creating a new one", "secret", secretName)

				caSecret, err := r.coreClient.Secrets("openshift-service-ca").Get(ctx, "signing-key", metav1.GetOptions{})
				if err != nil {
					reqLogger.Error(err, "Reading certificate authority signing key failed")
					return reconcile.Result{}, err
				}

				commonName := hawtio.Spec.Auth.ClientCertCommonName
				if commonName == "" {
					if r.ClientCertCommonName == "" {
						commonName = "hawtio-online.hawtio.svc"
					} else {
						commonName = r.ClientCertCommonName
					}
				}
				// Let's default to one year validity period
				expirationDate := time.Now().AddDate(1, 0, 0)
				if date := hawtio.Spec.Auth.ClientCertExpirationDate; date != nil && !date.IsZero() {
					expirationDate = date.Time
				}
				certSecret, err := generateCertificateSecret(secretName, request.Namespace, caSecret, commonName, expirationDate)
				if err != nil {
					reqLogger.Error(err, "Generating the client certificate failed")
					return reconcile.Result{}, err
				}
				err = controllerutil.SetControllerReference(hawtio, certSecret, r.scheme)
				if err != nil {
					return reconcile.Result{}, err
				}
				_, err = r.coreClient.Secrets(request.Namespace).Create(ctx, certSecret, metav1.CreateOptions{})
				reqLogger.Info("Client certificate created successfully", "secret", secretName)
				if err != nil {
					reqLogger.Error(err, "Creating the client certificate secret failed")
					return reconcile.Result{}, err
				}

				// check if certificate rotation is enabled
				if hawtio.Spec.Auth.ClientCertCheckSchedule != "" {
					// generate auto-renewal cron job for the secret if it already hasn't been generated.
					if cronJobErr != nil && errors.IsNotFound(cronJobErr) {
						pod, err := getOperatorPod(ctx, r.client, request.Namespace)
						if err != nil {
							return reconcile.Result{}, err
						}
						//create cronJob to validate the Cert
						cronJob = createCertValidationCronJob(cronJobName, request.Namespace,
							pod.Spec.Containers[0].Image, hawtio.Spec.Auth.ClientCertCheckSchedule,
							hawtio.Spec.Auth.ClientCertExpirationPeriod)

						err = controllerutil.SetControllerReference(hawtio, cronJob, r.scheme)
						if err != nil {
							return reconcile.Result{}, err
						}
						err = r.client.Create(ctx, cronJob)

						if err != nil {
							log.Error(err, "Cronjob haven't been created")
							return reconcile.Result{}, err
						}
					}
				}
			} else {
				return reconcile.Result{}, err
			}
		}
	}

	if cm := hawtio.Spec.RBAC.ConfigMap; cm != "" {
		// Check that the ConfigMap exists
		var rbacConfigMap corev1.ConfigMap
		err := r.client.Get(ctx, types.NamespacedName{Namespace: request.Namespace, Name: cm}, &rbacConfigMap)
		if err != nil {
			if errors.IsNotFound(err) {
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

	configMap := &corev1.ConfigMap{}
	err = r.client.Get(ctx, request.NamespacedName, configMap)
	if err != nil {
		reqLogger.Error(err, "Failed to get config map")
		return reconcile.Result{}, err
	}

	secretName := request.Name + "-tls-proxying"
	var clientCertSecret = &corev1.Secret{}
	err = r.client.Get(ctx, client.ObjectKey{Namespace: request.Namespace, Name: secretName}, clientCertSecret)
	if err != nil {
		// clientCertSecret isn't used on openshift 3
		clientCertSecret = nil
	}

	_, err = r.reconcileDeployment(hawtio, isOpenShift4, openShiftSemVer.String(), openShiftConsoleUrl, hawtio.Spec.Version, configMap, clientCertSecret)
	if err != nil {
		reqLogger.Error(err, "Error reconciling deployment")
		return reconcile.Result{}, err
	}

	route := &routev1.Route{}
	err = r.client.Get(ctx, request.NamespacedName, route)
	if err != nil && errors.IsNotFound(err) {
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get route")
		return reconcile.Result{}, err
	}

	if isClusterDeployment {
		// Add OAuth client
		oauthClient := resources.NewOAuthClient(resources.OAuthClientName)
		err = r.client.Create(ctx, oauthClient)
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
	if isOpenShift4 && hawtio.Status.Phase == hawtiov1alpha1.HawtioPhaseInitialized {
		consoleLink := &consolev1.ConsoleLink{}
		err = r.client.Get(ctx, types.NamespacedName{Name: consoleLinkName}, consoleLink)
		if err != nil {
			if !errors.IsNotFound(err) {
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
			consoleLink = openshift.NewApplicationMenuLink(consoleLinkName, route, hawtconfig)
		} else if isOpenShift43Plus {
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
	if hawtio.Status.Phase != hawtiov1alpha1.HawtioPhaseDeployed {
		previous := hawtio.DeepCopy()
		hawtio.Status.Phase = hawtiov1alpha1.HawtioPhaseDeployed
		err = r.client.Status().Patch(ctx, hawtio, client.MergeFrom(previous))
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update phase: %v", err)
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Update phase

	hawtioCopy := hawtio.DeepCopy()

	deployment := &appsv1.Deployment{}
	err = r.client.Get(ctx, request.NamespacedName, deployment)
	if err != nil && errors.IsNotFound(err) {
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

	// Reconcile route URL into Hawtio status
	url := resources.GetRouteURL(route)
	hawtioCopy.Status.URL = url

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

	// Reconcile console link in OpenShift console
	if isOpenShift4 {
		consoleLink := &consolev1.ConsoleLink{}
		err = r.client.Get(ctx, types.NamespacedName{Name: consoleLinkName}, consoleLink)
		if err != nil {
			if errors.IsNotFound(err) {
				// If not found, create a console link
				if isClusterDeployment {
					consoleLink = openshift.NewApplicationMenuLink(consoleLinkName, route, hawtconfig)
				} else if isOpenShift43Plus {
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
				openshift.UpdateApplicationMenuLink(consoleLinkCopy, route, hawtconfig)
			} else if isOpenShift43Plus {
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
	cronJob := &v1beta1.CronJob{}
	cronJobName := request.Name + "-certificate-expiry-check"

	if cronJobErr := r.client.Get(ctx, client.ObjectKey{Namespace: request.Namespace, Name: cronJobName}, cronJob); cronJobErr == nil {
		update := false
		if hawtio.Spec.Auth.ClientCertCheckSchedule != "" {
			if cronJob.Spec.Schedule != hawtio.Spec.Auth.ClientCertCheckSchedule {
				cronJob.Spec.Schedule = hawtio.Spec.Auth.ClientCertCheckSchedule
				update = true
			}
			if update || updateExpirationPeriod(cronJob, hawtio.Spec.Auth.ClientCertExpirationPeriod) {
				err = r.client.Update(ctx, cronJob)

				if err != nil {
					log.Error(err, "CronJob haven't been updated")
				}
			}

			//if cronjob exists and ClientCertRotate is disabled, cronjob has to be deleted
		} else {
			err = r.client.Delete(ctx, cronJob)
		}
	}

	// Reconcile OAuth client
	// Do not use the default client whose cached informers require
	// permission to list cluster wide oauth clients
	// err = r.client.Get(ctx, types.NamespacedName{Name: resources.OAuthClientName}, oc)
	oc, err := r.oauthClient.OauthV1().OAuthClients().Get(ctx, resources.OAuthClientName, metav1.GetOptions{})
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
	// wait until the deployment is successful before deleting resources
	if isClusterDeployment {
		// First remove old URL from OAuthClient
		if resources.RemoveRedirectURIFromOauthClient(oc, hawtio.Status.URL) {
			err := r.client.Update(ctx, oc)
			if err != nil {
				reqLogger.Error(err, "Failed to reconcile OAuth client")
				return reconcile.Result{}, err
			}
		}
		// Add route URL to OAuthClient authorized redirect URIs
		if ok, _ := resources.OauthClientContainsRedirectURI(oc, url); !ok {
			oc.RedirectURIs = append(oc.RedirectURIs, url)
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
		if resources.RemoveRedirectURIFromOauthClient(oc, url) {
			err := r.client.Update(ctx, oc)
			if err != nil {
				reqLogger.Error(err, "Failed to reconcile OAuth client")
				return reconcile.Result{}, err
			}
		}
	}

	err = r.client.Status().Patch(ctx, hawtioCopy, client.MergeFrom(hawtio))
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to patch status: %v", err)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileHawtio) reconcileConfigMap(hawtio *hawtiov1alpha1.Hawtio) (bool, error) {
	configMap, err := resources.NewConfigMap(hawtio)
	if err != nil {
		return false, err
	}
	return r.reconcileResources(hawtio, []resource.KubernetesResource{configMap}, []runtime.Object{&corev1.ConfigMapList{}})
}

func (r *ReconcileHawtio) reconcileDeployment(hawtio *hawtiov1alpha1.Hawtio, isOpenShift4 bool, openShiftVersion string, openShiftConsoleURL string, hawtioVersion string, configMap *corev1.ConfigMap, clientCertSecret *corev1.Secret) (bool, error) {
	clientCertSecretVersion := ""
	if clientCertSecret != nil {
		clientCertSecretVersion = clientCertSecret.GetResourceVersion()
	}
	deployment, err := resources.NewDeployment(hawtio, isOpenShift4, openShiftVersion, openShiftConsoleURL, hawtioVersion, configMap.GetResourceVersion(), clientCertSecretVersion, r.BuildVariables)
	if err != nil {
		return false, err
	}

	service := resources.NewService(hawtio.Name)
	route := resources.NewRoute(hawtio)

	var serviceAccount *corev1.ServiceAccount
	if hawtio.Spec.Type == hawtiov1alpha1.NamespaceHawtioDeploymentType {
		// Add service account as OAuth client
		serviceAccount, err = resources.NewServiceAccountAsOauthClient(hawtio.Name)
		if err != nil {
			return false, fmt.Errorf("error UpdateResources : %s", err)
		}
	}

	return r.reconcileResources(hawtio,
		[]resource.KubernetesResource{deployment, service, route, serviceAccount},
		[]runtime.Object{
			&corev1.ServiceList{},
			&appsv1.DeploymentList{},
			&routev1.RouteList{},
			&corev1.ServiceAccountList{},
		},
	)
}

func (r *ReconcileHawtio) reconcileResources(hawtio *hawtiov1alpha1.Hawtio, requestedResources []resource.KubernetesResource, listObjects []runtime.Object) (bool, error) {
	reqLogger := log.WithName(hawtio.Name)

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

func getDeployedResources(hawtio *hawtiov1alpha1.Hawtio, client client.Client, listObjects []runtime.Object) (map[reflect.Type][]resource.KubernetesResource, error) {
	reader := read.New(client).WithNamespace(hawtio.Namespace).WithOwnerObject(hawtio)
	resourceMap, err := reader.ListAll(listObjects...)
	if err != nil {
		log.Error(err, "Failed to list deployed objects")
		return nil, err
	}

	return resourceMap, nil
}

func (r *ReconcileHawtio) deletion(ctx context.Context, hawtio *hawtiov1alpha1.Hawtio) error {
	if controllerutil.ContainsFinalizer(hawtio, "foregroundDeletion") {
		return nil
	}

	if hawtio.Spec.Type == hawtiov1alpha1.ClusterHawtioDeploymentType {
		// Remove URI from OAuth client
		oc := &oauthv1.OAuthClient{}
		err := r.client.Get(ctx, types.NamespacedName{Name: resources.OAuthClientName}, oc)
		if err != nil && !errors.IsNotFound(err) {
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
	if err != nil && !errors.IsNotFound(err) && !meta.IsNoMatchError(err) {
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
