package resources

import (
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"

	hawtiov1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1"
)

const (
	HawtioTypeEnvVar                = "HAWTIO_ONLINE_MODE"
	HawtioNamespaceEnvVar           = "HAWTIO_ONLINE_NAMESPACE"
	HawtioAuthEnvVar                = "HAWTIO_ONLINE_AUTH"
	HawtioOAuthClientEnvVar         = "HAWTIO_OAUTH_CLIENT_ID"
	HawtioRbacEnvVar                = "HAWTIO_ONLINE_RBAC_ACL"
	HawtioGatewayEnvVar             = "HAWTIO_ONLINE_GATEWAY"
	HawtioDisableRbacRegistry       = "HAWTIO_ONLINE_DISABLE_RBAC_REGISTRY"
	OpenShiftClusterVersionEnvVar   = "OPENSHIFT_CLUSTER_VERSION"
	OpenShiftWebConsoleUrlEnvVar    = "OPENSHIFT_WEB_CONSOLE_URL"
	NginxClientBodyBufferSize       = "NGINX_CLIENT_BODY_BUFFER_SIZE"
	NginxProxyBuffers               = "NGINX_PROXY_BUFFERS"
	NginxSubrequestOutputBufferSize = "NGINX_SUBREQUEST_OUTPUT_BUFFER_SIZE"
	HawtioAuthTypeForm              = "form"
	HawtioAuthTypeOAuth             = "oauth"
)

func envVarsForHawtio(deploymentType hawtiov1.HawtioDeploymentType, name string, isOpenShift bool) []corev1.EnvVar {
	oauthClientId := name
	if deploymentType == hawtiov1.ClusterHawtioDeploymentType {
		// Pin to a known name for cluster-wide OAuthClient
		oauthClientId = OAuthClientName
	}

	envVars := []corev1.EnvVar{
		{
			Name:  HawtioTypeEnvVar,
			Value: strings.ToLower(string(deploymentType)),
		},
		{
			Name:  HawtioOAuthClientEnvVar,
			Value: oauthClientId,
		},
	}

	// Ensure that we provide the correct mode of authentication
	var authType string
	if isOpenShift {
		authType = HawtioAuthTypeOAuth
	} else {
		authType = HawtioAuthTypeForm
	}
	authTypeEnvVar := corev1.EnvVar{
		Name:  HawtioAuthEnvVar,
		Value: authType,
	}
	envVars = append(envVars, authTypeEnvVar)

	if deploymentType == hawtiov1.NamespaceHawtioDeploymentType {
		envVars = append(envVars, corev1.EnvVar{
			Name: HawtioNamespaceEnvVar,
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

func envVarsForOpenshift4(openShiftVersion string, openShiftConsoleURL string) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  HawtioGatewayEnvVar,
			Value: "true",
		},
		{
			Name:  OpenShiftClusterVersionEnvVar,
			Value: openShiftVersion,
		},
		{
			Name:  OpenShiftWebConsoleUrlEnvVar,
			Value: openShiftConsoleURL,
		},
	}
	return envVars
}

func envVarsForRBAC(rbac hawtiov1.HawtioRBAC) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	aclPath := ""
	if rbac.ConfigMap != "" {
		aclPath = path.Join(rbacConfigMapVolumeMountPath, RBACConfigMapKey)
	}
	envVars = append(envVars, corev1.EnvVar{
		Name:  HawtioRbacEnvVar,
		Value: aclPath,
	})

	if rbac.DisableRBACRegistry != nil && *rbac.DisableRBACRegistry {
		envVars = append(envVars, corev1.EnvVar{
			Name:  HawtioDisableRbacRegistry,
			Value: "true",
		})
	}

	return envVars
}

func envVarsForNginx(nginx hawtiov1.HawtioNginx) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	if nginx.ClientBodyBufferSize != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  NginxClientBodyBufferSize,
			Value: nginx.ClientBodyBufferSize,
		})
	}
	if nginx.ProxyBuffers != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  NginxProxyBuffers,
			Value: nginx.ProxyBuffers,
		})
	}
	if nginx.SubrequestOutputBufferSize != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  NginxSubrequestOutputBufferSize,
			Value: nginx.SubrequestOutputBufferSize,
		})
	}
	return envVars
}
