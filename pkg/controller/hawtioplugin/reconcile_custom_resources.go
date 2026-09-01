package hawtioplugin

import (
	"context"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	hawtiopluginv1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtioplugin/v1"

	"github.com/hawtio/hawtio-operator/pkg/util"
)

// HawtioPluginName is the name of the single console-plugin
const HawtioPluginName = "hawtio-console-plugin"

func (r *ReconcileHawtioPlugin) reconcileHawtio(ctx context.Context) (bool, error) {
	r.Logger.V(util.DebugLogLevel).Info("Querying Hawtio custom resources")

	hawtioList := &hawtiov2.HawtioList{}
	err := r.Client.List(ctx, hawtioList)
	if err != nil {
		if kerrors.IsNotFound(err) {
			r.Logger.V(util.DebugLogLevel).Info("No Hawtio CRs found")
			return false, nil
		}
		// Error reading the object - requeue the request.
		return false, err
	}

	if len(hawtioList.Items) == 0 {
		// No hawtio CRs
		r.Logger.V(util.DebugLogLevel).Info("No Hawtio CRs found")
		return false, nil
	}

	enabled := false
	for _, h := range hawtioList.Items {
		if h.Spec.EnableHawtioPlugin != nil && *h.Spec.EnableHawtioPlugin {
			// First one that requires the console plugin
			enabled = true
			break
		}
	}

	return enabled, nil
}

func (r *ReconcileHawtioPlugin) fetchHawtioPlugin(ctx context.Context) (*hawtiopluginv1.HawtioPlugin, error) {
	r.Logger.V(util.DebugLogLevel).Info("Fetching the HawtioPlugin custom resource")

	pluginList := &hawtiopluginv1.HawtioPluginList{}
	err := r.Client.List(ctx, pluginList)
	if err != nil {
		if kerrors.IsNotFound(err) {
			r.Logger.V(util.DebugLogLevel).Info("No HawtioPlugin CR found")
			return nil, nil
		}
		// Error reading the object - requeue the request.
		return nil, err
	}

	if len(pluginList.Items) == 0 {
		r.Logger.V(util.DebugLogLevel).Info("No HawtioPlugin CR found")
		return nil, nil
	}

	hawtioPlugin := pluginList.Items[0]
	if len(pluginList.Items) > 1 {
		r.Logger.Info("Warning: Multiple HawtioPlugin CRs found in cluster",
			"count", len(pluginList.Items),
			"selected", hawtioPlugin.Name,
		)
	}

	return &hawtioPlugin, nil
}

func (r *ReconcileHawtioPlugin) createHawtioPlugin(ctx context.Context) error {
	hawtioPlugin := hawtiopluginv1.NewHawtioPlugin(HawtioPluginName)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, hawtioPlugin, func() error {
		// Intentionally empty: spec remains empty default
		return nil
	})

	if err != nil {
		return err
	}

	return nil
}
