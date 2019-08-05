package util

import (
	"k8s.io/apimachinery/pkg/runtime"

	apps "github.com/openshift/api/apps/v1"
)

func GetDeploymentConfig(objects []runtime.Object) *apps.DeploymentConfig {
	for _, object := range objects {
		if deploymentConfig, ok := object.(*apps.DeploymentConfig); ok {
			return deploymentConfig
		}
	}
	return nil
}
