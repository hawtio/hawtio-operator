package resources

import (
	corev1 "k8s.io/api/core/v1"
)

func addEnvVarForContainer(deploymentType string, oauthClientId string) []corev1.EnvVar {
	namespaceEnvVarSource := corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{
			APIVersion: "v1",
			FieldPath:  "metadata.namespace",
		},
	}

	envVarArray := []corev1.EnvVar{
		{
			"HAWTIO_ONLINE_MODE",
			deploymentType,
			nil,
		},
		{
			"HAWTIO_ONLINE_NAMESPACE",
			"",
			&namespaceEnvVarSource,
		},
		{
			"HAWTIO_OAUTH_CLIENT_ID",
			oauthClientId,
			nil,
		},
	}

	return envVarArray
}

func addEnvVarForOpenshift(openshiftVersion string, openshiftURL string) []corev1.EnvVar {
	envVarArray := []corev1.EnvVar{
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
	return envVarArray
}
