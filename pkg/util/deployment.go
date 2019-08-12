package util

import (
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/runtime"
)

func GetEnvVarByName(env []core.EnvVar, name string) (*core.EnvVar, int) {
	for i, v := range env {
		if v.Name == name {
			return &env[i], i
		}
	}
	return nil, -1
}

func GetVolumeMount(container core.Container, name string) (*core.VolumeMount, int) {
	for i, vm := range container.VolumeMounts {
		if vm.Name == name {
			return &container.VolumeMounts[i], i
		}
	}
	return nil, -1
}

func GetDeployment(objects []runtime.Object) *apps.Deployment {
	for _, object := range objects {
		if deployment, ok := object.(*apps.Deployment); ok {
			return deployment
		}
	}
	return nil
}
