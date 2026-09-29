package hawtioplugin

import (
	"context"
	"time"

	hawtiopluginv1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtioplugin/v1"
	"github.com/hawtio/hawtio-operator/pkg/cfg"
)

const MasterClientSecretName = "hawtio-online-plugin-tls-proxying"

const ServiceClientSecretName = "hawtio-console-plugin-tls-serving"

// Cap the maximum requeue time to 24 hours
// Maximum time to sleep before a requeue should take place
var maxRequeueTime = 24 * time.Hour

func (r *ReconcileHawtioPlugin) initDeploymentConfiguration(ctx context.Context, hawtioPlugin *hawtiopluginv1.HawtioPlugin, clientSecretName string, nextCheckIn time.Duration) (cfg.DeploymentConfiguration, error) {
	deploymentConfiguration := cfg.DeploymentConfiguration{}

	//
	// Assign the client certificate
	// - used by the create client secret
	// - used by the deployment for its client certificate secret volume
	//
	deploymentConfiguration.ClientCertSecretName = &clientSecretName

	//
	// Assign the serving certificate name to match that in the service & deployment secret
	// - used by the service for its beta certificate annotation / generated secret
	// - used by the deployment for its client certificate secret volume
	//
	srvSecretName := ServiceClientSecretName
	deploymentConfiguration.ServingCertSecretName = &srvSecretName

	//
	// Assign the nextCheckIn as the RequeueAfter property
	//
	deploymentConfiguration.RequeueAfter = nextCheckIn

	return deploymentConfiguration, nil
}
