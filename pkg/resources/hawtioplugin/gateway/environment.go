package gateway

import (
	corev1 "k8s.io/api/core/v1"

	hawtiopluginv1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtioplugin/v1"
	common "github.com/hawtio/hawtio-operator/pkg/resources/common"
)

func envVarsForGateway(hawtioPlugin *hawtiopluginv1.HawtioPlugin) []corev1.EnvVar {

	//
	// Do not need to specify the following as assumed in image
	// as default path where secret volume is mounted
	// - HAWTIO_ONLINE_GATEWAY_SSL_PROXY_KEY
	// - HAWTIO_ONLINE_GATEWAY_SSL_PROXY_CERTIFICATE
	//
	// TODO:
	// - Why are the serving certificates not assumed as well?
	//   - HAWTIO_ONLINE_GATEWAY_SSL_KEY
	//   - HAWTIO_ONLINE_GATEWAY_SSL_CERTIFICATE
	//   - HAWTIO_ONLINE_GATEWAY_SSL_CERTIFICATE_CA
	//

	/*
	 * - name: HAWTIO_ONLINE_GATEWAY_SSL_KEY
	 *   value: /etc/tls/private/serving/tls.key
	 * - name: HAWTIO_ONLINE_GATEWAY_SSL_CERTIFICATE
	 *   value: /etc/tls/private/serving/tls.crt
	 * - name: HAWTIO_ONLINE_GATEWAY_SSL_CERTIFICATE_CA
	 *   value: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
	 */

	envVars := []corev1.EnvVar{
		corev1.EnvVar{
			Name:  common.GatewaySSLKeyEnvVar,
			Value: common.HawtioSSLKeyValue, // serving-certificate key
		},
		corev1.EnvVar{
			Name:  common.GatewaySSLCertEnvVar,
			Value: common.HawtioSSLCertValue, // serving-certificate certificate
		},
		corev1.EnvVar{
			Name:  common.GatewaySSLCertCAEnvVar,
			Value: common.HawtioSSLCertCAValue, // serviceaccount certificate authority
		},
	}

	/*
	 * - name: HAWTIO_ONLINE_GATEWAY_LOG_LEVEL
	 *   value: info
	 */
	gatewayLogLvl := common.GatewayLogLvlValue
	if hawtioPlugin.Spec.Logging.GatewayLogLevel != "" {
		gatewayLogLvl = hawtioPlugin.Spec.Logging.GatewayLogLevel
	}

	envVars = append(envVars,
		corev1.EnvVar{
			Name:  common.GatewayLogLvlEnvVar,
			Value: gatewayLogLvl,
		},
	)

	/*
	 * - name: HAWTIO_ONLINE_RBAC_ACL
	 *   value: /etc/hawtio/rbac/ACL.yaml
	 */

	envVarsForRBAC := common.EnvVarsForRBAC(hawtioPlugin.GetRbacOptions())
	envVars = append(envVars, envVarsForRBAC...)

	return envVars
}
