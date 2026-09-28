package common

import (
	corev1 "k8s.io/api/core/v1"
)

/*
 * Deployment Values
 */
const (
	/*
	 * Serving Certificate
	 */
	ServiceSigningSecretVolumeMountPath       = "/etc/tls/private/serving"
	ServiceSigningSecretVolumeMountPathLegacy = "/etc/tls/private"

	/*
	 * Proxy Certificate
	 */
	ClientCertificateSecretVolumeMountPath = "/etc/tls/private/proxying"
	ClientCertSecretVersionAnnotation      = "hawtio.hawt.io/certversion"

	/*
	 * RBAC
	 */
	RbacConfigMapVolumeName      = "hawtio-rbac"
	RbacConfigMapVolumeMountPath = "/etc/hawtio/rbac"
	RbacConfigMapKey             = "ACL.yaml"

	// OnlineReadinessPeriodValue is the default for Online's Readiness period value
	OnlineReadinessPeriodValue = 5
	// OnlineLivenessPeriodValue is the default for Online's Liveness period value
	OnlineLivenessPeriodValue = 10
	// GatewayReadinessPeriodValue is the default for Gateway's Readiness period value
	GatewayReadinessPeriodValue = 30
	// GatewayLivenessPeriodValue is the default for Gateway's Liveness period value
	GatewayLivenessPeriodValue = 120

	ConfigVersionAnnotation = "hawtio.hawt.io/configversion"
	ServerRootDirectory     = "/usr/share/nginx/html"

	ContainerPortName        = "nginx"
	ContainerGatewayPortName = "express"

	// PlainServicePort is the non-SSL server port
	PlainServicePort = 80
	// SSLServicePort is the SSL server port
	SSLServicePort = 443
)

// Connect encapsulates the properties of an http(s) connection
type Connect struct {
	Port     int32
	Protocol corev1.URIScheme
}

// PlainConnect is the properties of a plain (non-ssl) http connection
var PlainConnect = Connect{
	Port:     8080,
	Protocol: "HTTP",
}

// SSLConnect is the properties of an SSL (TLS encrypted) https connection
var SSLConnect = Connect{
	Port:     8443,
	Protocol: "HTTPS",
}

/*
 * Environment Variables
 */
const (
	/*
	 * Hawtio Server
	 */
	HawtioDisableRbacRegistry       = "HAWTIO_ONLINE_DISABLE_RBAC_REGISTRY"
	NginxClientBodyBufferSize       = "NGINX_CLIENT_BODY_BUFFER_SIZE"
	NginxProxyBuffers               = "NGINX_PROXY_BUFFERS"
	NginxSubrequestOutputBufferSize = "NGINX_SUBREQUEST_OUTPUT_BUFFER_SIZE"
	NginxMasterBurstSizeEnvVar      = "NGINX_MASTER_BURST"
	HawtioSSLKey                    = "HAWTIO_ONLINE_SSL_KEY"
	HawtioSSLCert                   = "HAWTIO_ONLINE_SSL_CERTIFICATE"

	/*
	 * Gateway Server
	 */
	GatewaySSLKeyEnvVar      = "HAWTIO_ONLINE_GATEWAY_SSL_KEY"            // /etc/tls/private/serving/tls.key
	GatewaySSLCertEnvVar     = "HAWTIO_ONLINE_GATEWAY_SSL_CERTIFICATE"    // /etc/tls/private/serving/tls.crt
	GatewaySSLCertCAEnvVar   = "HAWTIO_ONLINE_GATEWAY_SSL_CERTIFICATE_CA" // /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
	GatewayLogLvlEnvVar      = "HAWTIO_ONLINE_GATEWAY_LOG_LEVEL"          // info
	GatewayRbacEnvVar        = "HAWTIO_ONLINE_RBAC_ACL"
	GatewayMaskIPEnvVar      = "HAWTIO_ONLINE_MASK_IP_ADDRESSES" // true
	HawtioOnlineLogLvlEnvVar = "HAWTIO_ONLINE_LOG_LEVEL"         // info

	HawtioSSLKeyValue       = "/etc/tls/private/serving/tls.key"
	HawtioSSLCertValue      = "/etc/tls/private/serving/tls.crt"
	HawtioSSLCertCAValue    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	HawtioOnlineLogLvlValue = "info"
	GatewayLogLvlValue      = "info"
	GatewayMaskIPValue      = "false"
)
