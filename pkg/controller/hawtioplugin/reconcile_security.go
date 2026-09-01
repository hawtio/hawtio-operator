package hawtioplugin

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"

	hawtiopluginv1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtioplugin/v1"

	resources "github.com/hawtio/hawtio-operator/pkg/resources"
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
