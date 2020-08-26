package resources

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
)

const (
	hawtioTypeEnvVar        = "HAWTIO_ONLINE_MODE"
	hawtioNamespaceEnvVar   = "HAWTIO_ONLINE_NAMESPACE"
	hawtioOAuthClientEnvVar = "HAWTIO_OAUTH_CLIENT_ID"

	OAuthClientName = "hawtio"
)

func addEnvVarForContainer(deploymentType string, name string) []corev1.EnvVar {
	oauthClientId := name
	if strings.EqualFold(deploymentType, hawtiov1alpha1.ClusterHawtioDeploymentType) {
		// Pin to a known name for cluster-wide OAuthClient
		oauthClientId = OAuthClientName
	}

	envVars := []corev1.EnvVar{
		{
			Name:  hawtioTypeEnvVar,
			Value: strings.ToLower(deploymentType),
		},
		{
			Name:  hawtioOAuthClientEnvVar,
			Value: oauthClientId,
		},
	}

	if strings.EqualFold(deploymentType, hawtiov1alpha1.NamespaceHawtioDeploymentType) {
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
