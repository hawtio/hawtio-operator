package environments

import (
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("package environments")

func AddEnvVarForContainer(deploymentType string, OuthClient string) []corev1.EnvVar {

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
			OuthClient,
			nil,
		},
	}

	return envVarArray
}

func AddEnvVarForOpenshift(openshiftVersion string, openshiftURL string) []corev1.EnvVar {

	// Activate console backend gateway
	envVarArray := []corev1.EnvVar{
		{
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
