package hawtioplugin

import (
	"context"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"

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
