package resources

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"

	common "github.com/hawtio/hawtio-operator/pkg/resources/common"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

const (
	// HawtioTypeEnvVar is the env var for the mode of hawtio-online (exported for controller tests)
	HawtioTypeEnvVar = "HAWTIO_ONLINE_MODE"
	// HawtioNamespaceEnvVar is the env var for the namespace targetted by hawtio-online (exported for controller tests)
	HawtioNamespaceEnvVar = "HAWTIO_ONLINE_NAMESPACE"
	// HawtioOAuthClientEnvVar is the env var for the auth client-id of hawtio-online (exported for controller tests)
	HawtioOAuthClientEnvVar = "HAWTIO_OAUTH_CLIENT_ID"
	// HawtioAuthEnvVar is the env var for the type of authentication of hawtio-online (exported for controller tests)
	HawtioAuthEnvVar = "HAWTIO_ONLINE_AUTH"

	hawtioAuthTypeForm  = "form"
	hawtioAuthTypeOAuth = "oauth"

	openShiftClusterVersionEnvVar = "OPENSHIFT_CLUSTER_VERSION"
	openShiftWebConsoleUrlEnvVar  = "OPENSHIFT_WEB_CONSOLE_URL"
)

func envVarForAuth(isOpenShift bool) corev1.EnvVar {
	// Ensure that we provide the correct mode of authentication
	var authType string
	if isOpenShift {
		authType = hawtioAuthTypeOAuth
	} else {
		authType = hawtioAuthTypeForm
	}
	authTypeEnvVar := corev1.EnvVar{
		Name:  HawtioAuthEnvVar,
		Value: authType,
	}

	return authTypeEnvVar
}

func envVarsForHawtio(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec) []corev1.EnvVar {
	/*
	 * oauthClientId used in 2 configurations:
	 * 1. In namespace mode, service account with same name as Hawtio CR has annotations
	 *    for correct redirect. Redirect URI is generated from HAWTIO_OAUTH_CLIENT_ID
	 *    env var so need to specify CR name as that env var's value
	 *
	 * 2. In cluster mode, a separate oauthClient resource is created (see oauthclient.go).
	 *    This resource is given a default namespaced name.
	 */

	// HAWTIO_OAUTH_CLIENT_ID given CR name by default
	oauthClientID := hawtio.Name
	if hawtio.Spec.Type == hawtiov2.ClusterHawtioDeploymentType {
		// Update HAWTIO_OAUTH_CLIENT_ID to known namespaced name for cluster-wide OAuthClient
		oauthClientID = hawtio.Name + "-" + hawtio.Namespace
	}

	envVars := []corev1.EnvVar{
		{
			Name:  HawtioTypeEnvVar,
			Value: strings.ToLower(string(hawtio.Spec.Type)),
		},
	}

	if apiSpec.IsOpenShift4 {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  HawtioOAuthClientEnvVar,
				Value: oauthClientID,
			},
		)
	}

	isSSL := util.IsSSL(hawtio, apiSpec)
	if isSSL {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  common.HawtioSSLKey,
				Value: common.HawtioSSLKeyValue,
			},
			corev1.EnvVar{
				Name:  common.HawtioSSLCert,
				Value: common.HawtioSSLCertValue,
			},
		)
	}

	authTypeEnvVar := envVarForAuth(apiSpec.IsOpenShift4)
	envVars = append(envVars, authTypeEnvVar)

	if hawtio.Spec.Type == hawtiov2.NamespaceHawtioDeploymentType {
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

	onlineLogLvl := common.HawtioOnlineLogLvlValue
	if hawtio.Spec.Logging.OnlineLogLevel != "" {
		onlineLogLvl = hawtio.Spec.Logging.OnlineLogLevel
	}

	envVars = append(envVars,
		corev1.EnvVar{
			Name:  common.HawtioOnlineLogLvlEnvVar,
			Value: onlineLogLvl,
		},
	)

	return envVars
}

func envVarsForHawtioOCP4(openShiftVersion string, openShiftConsoleURL string) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  openShiftClusterVersionEnvVar,
			Value: openShiftVersion,
		},
		{
			Name:  openShiftWebConsoleUrlEnvVar,
			Value: openShiftConsoleURL,
		},
	}
	return envVars
}

func envVarsForGateway(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}
	isSSL := util.IsSSL(hawtio, apiSpec)
	if isSSL {
		envVars = append(envVars,
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
		)
	}

	gatewayLogLvl := common.GatewayLogLvlValue
	if hawtio.Spec.Logging.GatewayLogLevel != "" {
		gatewayLogLvl = hawtio.Spec.Logging.GatewayLogLevel
	}

	gatewayMaskIP := common.GatewayMaskIPValue
	if hawtio.Spec.Logging.MaskIPAddresses != "" {
		gatewayMaskIP = hawtio.Spec.Logging.MaskIPAddresses
	}

	envVars = append(envVars,
		corev1.EnvVar{
			Name:  common.GatewayLogLvlEnvVar,
			Value: gatewayLogLvl,
		},
		corev1.EnvVar{
			Name:  common.GatewayMaskIPEnvVar,
			Value: gatewayMaskIP,
		},
	)

	// Needs to be added to gateway in the same way as the hawtio image
	authTypeEnvVar := envVarForAuth(apiSpec.IsOpenShift4)
	envVars = append(envVars, authTypeEnvVar)

	return envVars
}
