package resources

import (
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	hawtiov1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1"
)

const containerPortName = "https"

func newContainer(hawtio *hawtiov1.Hawtio, envVars []corev1.EnvVar, imageVersion string, imageRepository string) corev1.Container {
	container := corev1.Container{
		Name:  hawtio.Name + "-container",
		Image: getImageFor(imageVersion, imageRepository),
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

func getImageFor(tag string, imageRepository string) string {
	repository := os.Getenv("IMAGE_REPOSITORY")
	if repository == "" {
		if imageRepository != "" {
			repository = imageRepository
		} else {
			repository = "quay.io/hawtio/online"
		}
	}

	if strings.HasPrefix(tag, "sha256:") {
		// tag is a sha checksum tag
		return repository + "@" + tag
	}

	return repository + ":" + tag
}
