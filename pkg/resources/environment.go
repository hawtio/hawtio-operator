package resources

import (
	"fmt"
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
)

const (
	HawtioTypeEnvVar                = "HAWTIO_ONLINE_MODE"
	HawtioNamespaceEnvVar           = "HAWTIO_ONLINE_NAMESPACE"
	HawtioAuthEnvVar                = "HAWTIO_ONLINE_AUTH"
	HawtioOAuthClientEnvVar         = "HAWTIO_OAUTH_CLIENT_ID"
	HawtioDisableRbacRegistry       = "HAWTIO_ONLINE_DISABLE_RBAC_REGISTRY"
	OpenShiftClusterVersionEnvVar   = "OPENSHIFT_CLUSTER_VERSION"
	OpenShiftWebConsoleUrlEnvVar    = "OPENSHIFT_WEB_CONSOLE_URL"
	NginxClientBodyBufferSize       = "NGINX_CLIENT_BODY_BUFFER_SIZE"
	NginxProxyBuffers               = "NGINX_PROXY_BUFFERS"
	NginxSubrequestOutputBufferSize = "NGINX_SUBREQUEST_OUTPUT_BUFFER_SIZE"
	HawtioAuthTypeForm              = "form"
	HawtioAuthTypeOAuth             = "oauth"
	HawtioSSLKey                    = "HAWTIO_ONLINE_SSL_KEY"
	HawtioSSLCert                   = "HAWTIO_ONLINE_SSL_CERTIFICATE"

	/*
	 * Gateway Env Vars
	 */
	GatewayWebSvrEnvVar    = "HAWTIO_ONLINE_GATEWAY_WEB_SERVER"         // https://localhost:8443
	GatewaySSLKeyEnvVar    = "HAWTIO_ONLINE_GATEWAY_SSL_KEY"            // /etc/tls/private/serving/tls.key
	GatewaySSLCertEnvVar   = "HAWTIO_ONLINE_GATEWAY_SSL_CERTIFICATE"    // /etc/tls/private/serving/tls.crt
	GatewaySSLCertCAEnvVar = "HAWTIO_ONLINE_GATEWAY_SSL_CERTIFICATE_CA" // /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
	GatewayRbacEnvVar      = "HAWTIO_ONLINE_RBAC_ACL"

	HawtioSSLKeyValue    = "/etc/tls/private/serving/tls.key"
	HawtioSSLCertValue   = "/etc/tls/private/serving/tls.crt"
	HawtioSSLCertCAValue = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

func envVarForAuth(isOpenShift bool) corev1.EnvVar {
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

	return authTypeEnvVar
}

func envVarsForHawtio(deploymentType hawtiov2.HawtioDeploymentType, name string, isOpenShift bool, isSSL bool) []corev1.EnvVar {

	envVars := []corev1.EnvVar{
		{
			Name:  HawtioTypeEnvVar,
			Value: strings.ToLower(string(deploymentType)),
		},
	}

	if isOpenShift && deploymentType == hawtiov2.ClusterHawtioDeploymentType {
		// Pin to a known name for cluster-wide OAuthClient
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  HawtioOAuthClientEnvVar,
				Value: OAuthClientName,
			},
		)
	}

	if isSSL {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  HawtioSSLKey,
				Value: HawtioSSLKeyValue,
			},
			corev1.EnvVar{
				Name:  HawtioSSLCert,
				Value: HawtioSSLCertValue,
			},
		)
	}

	authTypeEnvVar := envVarForAuth(isOpenShift)
	envVars = append(envVars, authTypeEnvVar)

	if deploymentType == hawtiov2.NamespaceHawtioDeploymentType {
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

func envVarsForHawtioOCP4(openShiftVersion string, openShiftConsoleURL string) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
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

func envVarsForNginx(nginx hawtiov2.HawtioNginx) []corev1.EnvVar {
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

func envVarsForGateway(isOpenShift bool, isSSL bool) []corev1.EnvVar {

	webSrvProtocol := "http"
	webSvrPort := 8080
	if isSSL {
		webSrvProtocol = "https"
		webSvrPort = 8443
	}

	envVars := []corev1.EnvVar{
		{
			Name:  GatewayWebSvrEnvVar,
			Value: fmt.Sprintf("%s://localhost:%d", webSrvProtocol, webSvrPort), // Same port as defined in hawtio container
		},
	}

	if isSSL {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  GatewaySSLKeyEnvVar,
				Value: HawtioSSLKeyValue, // serving-certificate key
			},
			corev1.EnvVar{
				Name:  GatewaySSLCertEnvVar,
				Value: HawtioSSLCertValue, // serving-certificate certificate
			},
			corev1.EnvVar{
				Name:  GatewaySSLCertCAEnvVar,
				Value: HawtioSSLCertCAValue, // serviceaccount certificate authority
			},
		)
	}

	// Needs to be added to gateway in the same way as the hawtio image
	authTypeEnvVar := envVarForAuth(isOpenShift)
	envVars = append(envVars, authTypeEnvVar)

	return envVars
}

func envVarsForRBAC(rbac hawtiov2.HawtioRBAC) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	aclPath := ""
	if rbac.ConfigMap != "" {
		aclPath = path.Join(rbacConfigMapVolumeMountPath, RBACConfigMapKey)

		envVars = append(envVars, corev1.EnvVar{
			Name:  GatewayRbacEnvVar,
			Value: aclPath,
		})
	}

	if rbac.DisableRBACRegistry != nil && *rbac.DisableRBACRegistry {
		envVars = append(envVars, corev1.EnvVar{
			Name:  HawtioDisableRbacRegistry,
			Value: "true",
		})
	}

	return envVars
}
