package common

import (
	"path"

	corev1 "k8s.io/api/core/v1"

	commonapi "github.com/hawtio/hawtio-operator/pkg/apis/common"
)

// EnvVarsForRBAC provides env vars for the RBAC settings
func EnvVarsForRBAC(rbac commonapi.RbacOptions) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	aclPath := ""
	if rbac.ConfigMap != "" {
		aclPath = path.Join(RbacConfigMapVolumeMountPath, RbacConfigMapKey)

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

// EnvVarsForNginx provides env vars for the nginx tuning settings
func EnvVarsForNginx(nginx commonapi.NginxOptions) []corev1.EnvVar {
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
	if nginx.MasterBurstSize != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  NginxMasterBurstSizeEnvVar,
			Value: nginx.MasterBurstSize,
		})
	}
	return envVars
}
