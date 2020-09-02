package resources

import (
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
)

const containerPortName = "https"

//func newContainer
func newContainer(hawtio *hawtiov1alpha1.Hawtio, envVars []corev1.EnvVar, imageRepository string) corev1.Container {
	container := corev1.Container{
		Name:  hawtio.Name + "-container",
		Image: getImageFor(hawtio.Spec.Version, imageRepository),
		Env:   envVars,
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
		Resources: hawtio.Spec.Resources,
	}

	return container
}

//TODO(): will replace that function with update code committed in PR #23
func getImageFor(version string, imageRepository string) string {
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
