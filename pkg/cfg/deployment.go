package cfg

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// DeploymentConfiguration acquires properties used in deployment
type DeploymentConfiguration struct {
	OpenShiftConsoleURL  string
	ConfigMap            *corev1.ConfigMap
	ClientCertSecretName *string        // -proxying certificate secret name
	TLSRouteSecret       *corev1.Secret // custom route certificate secret
	CACertRouteSecret    *corev1.Secret // custom CA certificate secret
	ServingCertSecret    *corev1.Secret // -serving certificate secret
	RequeueAfter         time.Duration  // time until next required requeuing of reconciler
}
