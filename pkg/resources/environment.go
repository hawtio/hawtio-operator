package resources

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
)

const (
	hawtioTypeEnvVar      = "HAWTIO_ONLINE_MODE"
	hawtioNamespaceEnvVar = "HAWTIO_ONLINE_NAMESPACE"
)

func addEnvVarForContainer(deploymentType string, oauthClientId string) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  hawtioTypeEnvVar,
			Value: strings.ToLower(deploymentType),
		},
		{
			"HAWTIO_OAUTH_CLIENT_ID",
			oauthClientId,
			nil,
		},
	}

	if deploymentType == hawtiov1alpha1.NamespaceHawtioDeploymentType {
		envVars = append(envVars, corev1.EnvVar{
			Name: hawtioNamespaceEnvVar,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.namespace",
				},
			},
		})
	}

	return envVars
}

func addEnvVarForOpenshift(openshiftVersion string, openshiftURL string) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			// Activate console backend gateway
			Name:  "HAWTIO_ONLINE_GATEWAY",
			Value: "true",
		},
		{
			Name:  "OPENSHIFT_CLUSTER_VERSION",
			Value: openshiftVersion,
		},
		{
			Name:  "OPENSHIFT_WEB_CONSOLE_URL",
			Value: openshiftURL,
		},
	}
	return envVars
}
