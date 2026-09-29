package hawtioplugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	routev1 "github.com/openshift/api/route/v1"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	hawtiopluginv1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtioplugin/v1"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	"github.com/hawtio/hawtio-operator/pkg/clients"
	commonctrl "github.com/hawtio/hawtio-operator/pkg/controller/common"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

const (
	singletonHawtioPluginName = "hawtio-console-plugin"
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
	if os.Getenv(commonctrl.HawtioUnderTestEnvVar) == "true" {
		skipValidation = true
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&hawtiopluginv1.HawtioPlugin{}).
		WithOptions(controller.Options{
			SkipNameValidation: &skipValidation,
		}).
		Named("hawtioplugin-controller").
		Watches(
			hawtiov2.NewHawtio(),
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      singletonHawtioPluginName,
							Namespace: r.OperatorPod.Namespace,
						},
					},
				}
			}),
		).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&routev1.Route{}).
		Complete(r)
}

// Reconcile is responsible for processing the HawtioPlugin CR and other watched resources
func (r *ReconcileHawtioPlugin) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	r.Logger = pluginLogger.WithValues("Operator Namespace", r.OperatorPod.Namespace, "Request.Name", request.Name)
	r.Logger.Info("Reconciling cluster-level HawtioPlugin")

	r.Logger.V(util.DebugLogLevel).Info(fmt.Sprintf("Cluster API Specification: %+v", r.ApiSpec))

	// =====================================================================
	// PHASE 1: SETUP: FETCH HAWTIO
	// =====================================================================
	// Check all hawtio CRs to see whether console should be
	// installed or remain installed
	//
	r.Logger.V(util.DebugLogLevel).Info("=== Finding Aggregate Hawtio Plugin Enablement ===")
	shouldEnablePlugin, err := r.reconcileHawtio(ctx)
	r.Logger.V(util.DebugLogLevel).Info("Hawtio CR plugin", "enabled", shouldEnablePlugin)
	if err != nil {
		return reconcile.Result{}, err
	}

	// =====================================================================
	// PHASE 2: SETUP: FETCH HAWTIOPLUGIN CR
	// =====================================================================
	//
	// Find the HawtioPlugin CR if one exists
	// - If exists then validate the configuration
	// - If not then create a new default one and requeue
	//
	// Fetch the Hawtio instance from the cluster.
	r.Logger.V(util.DebugLogLevel).Info("=== Fetching Hawtio Plugin Custom Resource ===")
	hawtioPlugin, err := r.fetchHawtioPlugin(ctx)
	if err != nil {
		return reconcile.Result{}, err
	} else if hawtioPlugin == nil {
		// HawtioPlugin CR not found so create a new one
		err := r.createHawtioPlugin(ctx)
		// Requeue to re-evaluate
		return reconcile.Result{Requeue: true}, err
	}

	hawtioPluginStatus := createStatusWithImages(r.BuildVariables)

	// hawtioPlugin should exist, either as a default or user-defined CR

	if !shouldEnablePlugin {
		r.Logger.Info("Hawtio Console Plugin not required - disabling")
		//
		// No hawtio CR enablement at this time
		//
		// - Remove all resources
		//   * Delete all resources in reverse order using r.Delete
		//   * No need to requeue inbetween except if there is an error
		//   * Ignore NotFound errors:
		//     if err := r.Delete(ctx, service); client.IgnoreNotFound(err) != nil {
		//

		// TODO

		// - Change HawtioPlugin phase to Disabled
		// - Complete
		//
		r.Logger.V(util.DebugLogLevel).Info("Setting HawtioPlugin phase to Disabled")
		hawtioPluginStatus.Phase = hawtiopluginv1.PluginPhaseDisabled
		err := r.setPluginStatus(ctx, hawtioPlugin, hawtioPluginStatus)
		return reconcile.Result{}, err
	}

	// =====================================================================
	// PHASE 3: INITIALIZE STATUS
	// =====================================================================
	// Verify existence of optional resources - rbac configmap
	// Check the status of the RBAC ConfigMap.
	// If specified in the CR then it should be present.
	//
	// - verification of configmap to determine syntax is ok?
	//
	r.Logger.V(util.DebugLogLevel).Info("=== Verifying RBAC ConfigMap ===")
	valid, err := r.verifyRBACConfigMap(ctx, hawtioPlugin, r.OperatorPod.Namespace)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// Let's poll for the RBAC ConfigMap to be created
			return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
		}

		return reconcile.Result{}, err
	} else if !valid {
		// Lets poll until the RBAC ConfigMap is valid
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	}

	// Resolve the master client proxy certificate (if applicable)
	r.Logger.V(util.DebugLogLevel).Info("=== Resolving Plugin Console Client Certificate ===")
	masterClientSecret, nextMasterCertCheckIn, err := r.ResolveMasterClientCertificate(ctx, hawtioPlugin, MasterClientSecretName)
	if err != nil {
		return commonctrl.HandleResultAndError(err)
	}
	if masterClientSecret == nil {
		err := errors.New("master client certificate secret should be initialized")
		return commonctrl.HandleResultAndError(err)
	}

	r.Logger.V(util.DebugLogLevel).Info("Plugin Console Client Certificate", "secret", masterClientSecret.Name, "next check-in", nextMasterCertCheckIn.String())

	//
	// - If the rbac ConfigMap was invalid then fail (not found is ok)
	//
	if len(hawtioPlugin.Status.Phase) == 0 || hawtioPlugin.Status.Phase == hawtiopluginv1.PluginPhaseFailed {
		r.Logger.V(util.DebugLogLevel).Info("Hawtio.Status.Phase is zero or failed. Setting to initialized.")
		hawtioPluginStatus.Phase = hawtiopluginv1.PluginPhaseInitialized
		err := r.setPluginStatus(ctx, hawtioPlugin, hawtioPluginStatus)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{Requeue: true}, nil
	}

	// =====================================================================
	// PHASE 4: RECONCILE AND DEPLOY PHASE
	// =====================================================================

	// Reconcile the service account
	r.Logger.V(util.DebugLogLevel).Info("=== Reconciling Service Account ===")
	opResult, err := r.reconcileServiceAccount(ctx, hawtioPlugin, r.OperatorPod.Namespace)
	r.LogOperationResult("ServiceAccount", opResult)
	if err != nil {
		return commonctrl.HandleResultAndError(err)
	}

	// Intialize the deployment inputs required for the deployment resources
	// * proxy certificate
	r.Logger.V(util.DebugLogLevel).Info("=== Initializing Deployment Configuration ===")
	deploymentConfig, err := r.initDeploymentConfiguration(ctx, hawtioPlugin, masterClientSecret.Name, nextMasterCertCheckIn)
	if err != nil {
		return commonctrl.HandleResultAndError(err)
	}

	//2
	// - Plugin
	//   * ConsolePlugin: hawtio-online-console-plugin
	//   * Service: hawtio-online-console-plugin
	//   * Deployment: hawtio-online-console-plugin
	//   * service certificate secret generated from annotation in Service
	//
	// - Gateway
	//   * ConfigMap: hawtio-online-console-plugin-rbac
	//   * Deployment: hawtio-online-console-plugin-gateway
	//   * Service: hawtio-online-console-plugin-gateway (service certificate)
	//   * Route: hawtio-online-console-plugin-gateway
	//

	// TODO - resources

	// =====================================================================
	// PHASE 5: UPDATE PHASE
	// =====================================================================
	r.Logger.V(util.DebugLogLevel).Info("Update phase - refreshing HawtioPlugin status")

	//
	// Await the installation of the deployment
	//
	// TODO
	// deployment := &appsv1.Deployment{}
	// err = r.Client.Get(ctx, crNamespacedName, deployment)
	// if err != nil && kerrors.IsNotFound(err) {
	// 	return reconcile.Result{Requeue: true}, nil
	// } else if err != nil {
	// 	r.Logger.Error(err, "Failed to get deployment")
	// 	return reconcile.Result{}, err
	// }

	// Refresh the hawtioplugin CR to minimize conflict window
	// Gets the absolute latest ResourceVersion from the server.
	// TODO
	// if err := r.Client.Get(ctx, crNamespacedName, hawtioPlugin); err != nil {
	// 	return reconcile.Result{}, err
	// }

	// Create a copy of the status to modify.
	// TODO
	// newStatus := hawtioPlugin.Status.DeepCopy()
	//
	// Reconcile Hawtio status image field from deployment container image
	// newStatus.Image = deployment.Spec.Template.Spec.Containers[0].Image
	// newStatus.GatewayImage = deployment.Spec.Template.Spec.Containers[1].Image

	// Determine the overall phase based on the deployment's readiness.
	// if deployment.Status.ReadyReplicas > 0 {
	// 	newStatus.Phase = hawtiov2.HawtioPhaseDeployed
	// } else {
	// 	// The Deployment isn't ready yet. Let's check if it has timed out.
	// 	if r.isDeploymentFailed(deployment) {
	// 		newStatus.Phase = hawtiov2.HawtioPhaseFailed
	// 	} else {
	// 		// It's not ready, but it hasn't failed yet. It's likely pulling images
	// 		// or waiting for the container to start. Keep it as Initialized.
	// 		newStatus.Phase = hawtiov2.HawtioPhaseInitialized
	// 	}
	// }
	// Only send an update to the API server if the status has actually changed.
	// This prevents empty updates and reduces load on the API server.
	// if !reflect.DeepEqual(hawtio.Status, *newStatus) {
	// 	hawtio.Status = *newStatus
	// 	r.Logger.Info("Status has changed, updating Hawtio CR",
	// 		"Phase", newStatus.Phase,
	// 		"URL", newStatus.URL,
	// 		"Replicas", newStatus.Replicas,
	// 		"Image", newStatus.Image,
	// 		"Gateway Image", newStatus.GatewayImage)
	// 	if err := r.Client.Status().Update(ctx, hawtio); err != nil {
	// 		r.Logger.Error(err, "Failed to update Hawtio status")
	// 		return reconcile.Result{}, err
	// 	}
	// }

	if deploymentConfig.RequeueAfter > 0 {
		r.Logger.Info("Reconciliation complete. Scheduling next cert rotation check.", "WakeUpIn", deploymentConfig.RequeueAfter.String())
		return reconcile.Result{RequeueAfter: deploymentConfig.RequeueAfter}, nil
	}

	return reconcile.Result{}, nil
}
