package hawtioplugin

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/types"

	hawtiopluginv1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtioplugin/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	resources "github.com/hawtio/hawtio-operator/pkg/resources"
	pluginres "github.com/hawtio/hawtio-operator/pkg/resources/hawtioplugin"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

func (r *ReconcileHawtioPlugin) verifyRBACConfigMap(ctx context.Context, hawtioPlugin *hawtiopluginv1.HawtioPlugin, namespace string) (bool, error) {
	cm := hawtioPlugin.Spec.RBAC.ConfigMap
	if cm == "" {
		return true, nil // No RBAC configMap specified so default will be used
	}

	r.Logger.V(util.DebugLogLevel).Info("Checking Hawtio.Spec.RBAC config map is valid")

	// Check that the ConfigMap exists
	var rbacConfigMap corev1.ConfigMap

	// Use API Reader to bypass cache and obtain legacy resource
	err := r.ApiReader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cm}, &rbacConfigMap)
	if err != nil {
		r.Logger.Error(err, "Failed to get RBAC ConfigMap")
		return false, err
	}

	if _, ok := rbacConfigMap.Data[resources.RBACConfigMapKey]; !ok {
		r.Logger.Info("RBAC ConfigMap does not contain expected key: "+resources.RBACConfigMapKey, "ConfigMap", cm)
		// Let's poll for the RBAC ConfigMap to contain the expected key
		return false, nil
	}

	return true, nil
}

func (r *ReconcileHawtioPlugin) reconcileServiceAccount(ctx context.Context, hawtioPlugin *hawtiopluginv1.HawtioPlugin, namespace string) (controllerutil.OperationResult, error) {
	serviceAccount := pluginres.NewDefaultServiceAccount(namespace)

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, serviceAccount, func() error {
		// A read-only copy of the cluster state for diff logging
		liveSnapshot := serviceAccount.DeepCopy()

		// Set the owner reference for garbage collection.
		if err := controllerutil.SetControllerReference(hawtioPlugin, serviceAccount, r.Scheme); err != nil {
			return err
		}

		reqLogger := r.Logger.WithName(fmt.Sprintf("%s-reconcileServiceAccount", hawtioPlugin.Name))
		crServiceAccount, err := pluginres.NewServiceAccount(namespace, hawtioPlugin, reqLogger)
		if err != nil {
			return err
		}

		serviceAccount.Labels = util.MergeMap(serviceAccount.Labels, crServiceAccount.GetLabels())
		serviceAccount.Annotations = util.MergeMap(serviceAccount.Annotations, crServiceAccount.GetAnnotations())

		// Report any known differences to the log (only if in debug log level)
		util.ReportDiff("ServiceAccount", liveSnapshot, serviceAccount)

		return nil
	})

	// There is a resource but the default client cache cannot see it
	// since it is legacy and not labelled. The create failed so it
	// should be adopted and reconciled.
	if err != nil && kerrors.IsAlreadyExists(err) {
		adoptionErr := r.AdoptLegacyResource(ctx, pluginres.NewDefaultServiceAccount(namespace))
		if adoptionErr != nil {
			// If adoption failed (e.g., API error) or an adoption was called
			// return any of these errors
			return controllerutil.OperationResultNone, adoptionErr
		}
	}

	if err != nil {
		return opResult, err
	}

	util.ReportResourceChange("ServiceAccount", serviceAccount, opResult)
	return opResult, nil
}
