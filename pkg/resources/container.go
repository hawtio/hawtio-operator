package resources

import (
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const containerPortName = "https"

//func NewContainer
func NewContainer(name string, version string, envVarArray []corev1.EnvVar, imageRepository string) corev1.Container {
	container := corev1.Container{
		Name:  name + "-container",
		Image: getImageForCR(version, imageRepository),
		Env:   envVarArray,
		ReadinessProbe: &corev1.Probe{
			InitialDelaySeconds: 5,
			TimeoutSeconds:      1,
			PeriodSeconds:       5,
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.FromString(containerPortName),
					Path:   "/online",
					Scheme: "HTTPS",
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			TimeoutSeconds: 1,
			PeriodSeconds:  10,
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.FromString(containerPortName),
					Path:   "/online",
					Scheme: "HTTPS",
				},
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          containerPortName,
				ContainerPort: 8443,
				Protocol:      "TCP",
			},
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
		},
	}

	return container
}

//TODO(): will replace that function with update code committed in PR #23
func getImageForCR(version string, imageRepository string) string {
	tag := "latest"
	if len(version) > 0 {
		tag = version
	}
	repository := os.Getenv("IMAGE_REPOSITORY")
	if repository == "" {
		if imageRepository != "" {
			repository = imageRepository
		} else {
			repository = "docker.io/hawtio/online"
		}
	}

	return repository + ":" + tag
}
