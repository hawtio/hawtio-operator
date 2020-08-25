package containers

import (
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const timeInSeconds = 5

// Go build-time variables
var ImageRepository string

//func NewContainer
func NewContainer(customResourceName string, version string, envVarArray []corev1.EnvVar) corev1.Container {
	container := corev1.Container{
		Name:  customResourceName + "-container",
		Image: getImageForCR(version),
		Env:   envVarArray,
		ReadinessProbe: &corev1.Probe{
			InitialDelaySeconds: timeInSeconds,
			TimeoutSeconds:      1,
			PeriodSeconds:       timeInSeconds,
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.FromString("nginx"),
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
					Port:   intstr.FromString("nginx"),
					Path:   "/online",
					Scheme: "HTTPS",
				},
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "nginx",
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
func getImageForCR(version string) string {
	tag := "latest"
	if len(version) > 0 {
		tag = version
	}
	repository := os.Getenv("IMAGE_REPOSITORY")
	if repository == "" {
		if ImageRepository != "" {
			repository = ImageRepository
		} else {
			repository = "docker.io/hawtio/online"
		}
	}

	return repository + ":" + tag
}
