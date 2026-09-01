package hawtioplugin

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	hawtiopluginv1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtioplugin/v1"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

func createStatusWithImages(bv util.BuildVariables) hawtiopluginv1.HawtioPluginStatus {
	return hawtiopluginv1.HawtioPluginStatus{
		PluginImage:  bv.ImageURI(bv.GetPluginImageRepo(), bv.GetPluginVersion()),
		GatewayImage: bv.ImageURI(bv.GetPluginGatewayImageRepo(), bv.GetPluginGatewayVersion()),
	}
}

func (r *ReconcileHawtioPlugin) setPluginStatus(ctx context.Context, hawtioPlugin *hawtiopluginv1.HawtioPlugin, newStatus hawtiopluginv1.HawtioPluginStatus) error {
	r.Logger.V(util.DebugLogLevel).Info("Setting HawtioPlugin CR Status:", "status", newStatus)

	previous := hawtioPlugin.DeepCopy()
	hawtioPlugin.Status.Phase = newStatus.Phase
	hawtioPlugin.Status.PluginImage = newStatus.PluginImage
	hawtioPlugin.Status.GatewayImage = newStatus.GatewayImage
	err := r.Client.Status().Patch(ctx, hawtioPlugin, client.MergeFrom(previous))
	if err != nil {
		return fmt.Errorf("failed to update HawtioPlugin status: %v: %v", newStatus, err)
	}

	return nil
}
