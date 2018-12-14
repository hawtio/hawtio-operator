package hawtio

import (
	"context"
	"fmt"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	"github.com/hawtio/hawtio-operator/pkg/openshift"
	"github.com/hawtio/hawtio-operator/pkg/openshift/template"
	"github.com/hawtio/hawtio-operator/pkg/openshift/util"

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
	routev1 "github.com/openshift/api/route/v1"
	templatev1 "github.com/openshift/api/template/v1"
)

var log = logf.Log.WithName("controller_hawtio")

const (
	hawtioTemplatePath      = "templates/deployment-namespace.yaml"
	hawtioVersionAnnotation = "hawtio.hawt.io/hawtioversion"
	configVersionAnnotation = "hawtio.hawt.io/configversion"
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
	err = routev1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}

	r := &ReconcileHawtio{
		client: mgr.GetClient(),
		config: mgr.GetConfig(),
		scheme: mgr.GetScheme(),
	}

	processor, err := template.NewProcessor(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.template = processor

	deployment, err := openshift.NewDeploymentClient(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.deployment = deployment

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
	client     client.Client
	config     *rest.Config
	scheme     *runtime.Scheme
	template   *template.TemplateProcessor
	deployment *openshift.DeploymentClient
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

	err = r.createObjects(objs, request.Namespace, instance)
	if err != nil {
		reqLogger.Error(err, "Error creating runtime objects")
		return reconcile.Result{}, err
	}

	config := &corev1.ConfigMap{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Namespace: request.Namespace, Name: request.Name + "-config"}, config)
	if err != nil && errors.IsNotFound(err) {
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get config map")
		return reconcile.Result{}, err
	}

	deployment := &appsv1.DeploymentConfig{}
	err = r.client.Get(context.TODO(), request.NamespacedName, deployment)
	if err != nil && errors.IsNotFound(err) {
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get deployment")
		return reconcile.Result{}, err
	} else {
		updateDeployment := false

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
	}

	// TODO: reconcile route from CR
	route := &routev1.Route{}
	err = r.client.Get(context.TODO(), request.NamespacedName, route)
	if err != nil && errors.IsNotFound(err) {
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get route")
		return reconcile.Result{}, err
	} else {
		if url := util.GetRouteURL(route); instance.Status.URL != url {
			instance.Status.URL = url
			err := r.client.Status().Update(context.TODO(), instance)
			if err != nil {
				reqLogger.Error(err, "Failed to reconcile from route")
				return reconcile.Result{}, err
			}
		}
	}

	stream := &imagev1.ImageStream{}
	err = r.client.Get(context.TODO(), request.NamespacedName, stream)
	if err != nil && errors.IsNotFound(err) {
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get image stream")
		return reconcile.Result{}, err
	} else {
		image := "<invalid>"
		for _, tag := range stream.Spec.Tags {
			// TODO: use tag parameter
			if tag.Name == "latest" {
				image = tag.From.Name
			}
		}
		if instance.Status.Image != image {
			instance.Status.Image = image
			err := r.client.Status().Update(context.TODO(), instance)
			if err != nil {
				reqLogger.Error(err, "Failed to reconcile from image stream")
				return reconcile.Result{}, err
			}
		}
	}

	// refresh the instance
	instance = &hawtiov1alpha1.Hawtio{}
	err = r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		reqLogger.Error(err, "Failed to refresh CR")
		return reconcile.Result{}, err
	}
	// and report back the version into the owned deployment
	if annotations := deployment.GetAnnotations(); annotations != nil && annotations[hawtioVersionAnnotation] != instance.GetResourceVersion() {
		deployment.Annotations[hawtioVersionAnnotation] = instance.GetResourceVersion()
		err := r.client.Update(context.TODO(), deployment)
		if err != nil {
			reqLogger.Error(err, "Failed to refresh deployment owner version")
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileHawtio) processTemplate(cr *hawtiov1alpha1.Hawtio, request reconcile.Request) ([]runtime.RawExtension, error) {
	res, err := util.LoadKubernetesResourceFromFile(hawtioTemplatePath)
	if err != nil {
		return nil, fmt.Errorf("Error reading template: %s", err)
	}

	parameters := make(map[string]string)
	parameters["APPLICATION_NAME"] = cr.Name
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
		uo, err := util.UnstructuredFromRuntimeObject(o)
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

func getRuntimeObjects(exts []runtime.RawExtension) ([]runtime.Object, error) {
	objects := make([]runtime.Object, 0)

	for _, ext := range exts {
		res, err := util.LoadKubernetesResource(ext.Raw)
		if err != nil {
			return nil, err
		}
		objects = append(objects, res)
	}

	return objects, nil
}
