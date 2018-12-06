package hawtio

import (
	"context"
	"fmt"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	"github.com/hawtio/hawtio-operator/pkg/openshift/template"
	"github.com/hawtio/hawtio-operator/pkg/openshift/util"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	imagev1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	templatev1 "github.com/openshift/api/template/v1"
)

var log = logf.Log.WithName("controller_hawtio")

const (
	hawtioTemplatePath = "templates/deployment-namespace.yaml"
)

// Add creates a new Hawtio Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	err := imagev1.AddToScheme(mgr.GetScheme())
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
	err = c.Watch(&source.Kind{Type: &hawtiov1alpha1.Hawtio{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resources and requeue the owner Hawtio
	err = c.Watch(&source.Kind{Type: &routev1.Route{}}, &handler.EnqueueRequestForOwner{
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
	client client.Client
	config *rest.Config
	scheme *runtime.Scheme
	template *template.TemplateProcessor
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

	route := &routev1.Route{}
	err = r.client.Get(context.TODO(), request.NamespacedName, route)
	if err != nil && errors.IsNotFound(err) {
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get route")
		return reconcile.Result{}, err
	} else {
		url := util.GetRouteURL(route)
		if instance.Status.URL == url {
			// Avoid another CR reconcile cycle
			return reconcile.Result{}, nil
		}
		instance.Status.URL = url
		err := r.client.Update(context.TODO(), instance)
		if err != nil {
			reqLogger.Error(err, "Failed to update status")
			return reconcile.Result{}, err
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
		if instance.Status.Image == image {
			// Avoid another CR reconcile cycle
			return reconcile.Result{}, nil
		}
		instance.Status.Image = image
		err := r.client.Update(context.TODO(), instance)
		if err != nil {
			reqLogger.Error(err, "Failed to update status")
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
	if replicas := cr.Spec.ReplicaCount; replicas > 0 {
		parameters["REPLICA_COUNT"] = fmt.Sprint(replicas)
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

		err = r.client.Create(context.TODO(), uo.DeepCopyObject())
		if err != nil {
			if errors.IsAlreadyExists(err) {
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
