package util

import (
	"encoding/json"
	"fmt"
	"strings"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"

	oauthv1 "github.com/openshift/api/oauth/v1"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

// SensitiveKeys a list of keys to be
// removed from any resource
var SensitiveKeys = []string{
	"key",
	"tls.key",
	"password",
	"token",
	"clientsecret",
	"secret",
	"admin-password",
}

const REDACTED = "[REDACTED]"

// jsonString Convert an object to a json string
func jsonString(v interface{}) string {
	j, err := json.Marshal(v)
	if err != nil {
		log.Error(err, "Cannot convert to json")
		return "{}"
	}

	return string(j)
}

func isSensitive(s string) bool {
	sensitive := false
	for _, sensitiveKey := range SensitiveKeys {
		if strings.Contains(strings.ToLower(s), sensitiveKey) {
			sensitive = true
			break
		}
	}

	return sensitive
}

func redactRoute(route *routev1.Route) string {
	if route.Spec.TLS != nil && route.Spec.TLS.Key != "" {
		route.Spec.TLS.Key = REDACTED
	}
	return jsonString(route)
}

func redactPodSpec(spec *corev1.PodSpec) {
	for i := range spec.Containers {
		for j := range spec.Containers[i].Env {
			envName := spec.Containers[i].Env[j].Name
			if isSensitive(envName) {
				spec.Containers[i].Env[j].Value = "[REDACTED]"
			}
		}
	}
	// Do the same for init containers
	for i := range spec.InitContainers {
		for j := range spec.InitContainers[i].Env {
			if isSensitive(spec.InitContainers[i].Env[j].Name) {
				spec.InitContainers[i].Env[j].Value = "[REDACTED]"
			}
		}
	}
}

func redactOAuthClient(client *oauthv1.OAuthClient) string {
	if len(client.Secret) > 0 {
		client.Secret = REDACTED
	}

	if len(client.AdditionalSecrets) > 0 {
		for i := 0; i < len(client.AdditionalSecrets); i++ {
			client.AdditionalSecrets[i] = REDACTED
		}
	}

	return jsonString(client)
}

func redactIngress(ingress *networkingv1.Ingress) string {
	annotations := ingress.GetAnnotations()
	for k := range annotations {
		if isSensitive(k) {
			annotations[k] = REDACTED
		}
	}
	ingress.SetAnnotations(annotations)
	return jsonString(ingress)
}

func redactDeployment(deploy *appsv1.Deployment) string {
	redactPodSpec(&deploy.Spec.Template.Spec)
	return jsonString(deploy)
}

func redactConfigMap(configMap *corev1.ConfigMap) string {
	for key := range configMap.Data {
		if isSensitive(key) {
			configMap.Data[key] = REDACTED
		}
	}
	for key := range configMap.BinaryData {
		if isSensitive(key) {
			configMap.BinaryData[key] = []byte(REDACTED)
		}
	}

	return jsonString(configMap)
}

// ToJSONString inspects an object, redacts fields
// and returns a JSON string.
func ToJSONString(obj interface{}) string {
	if obj == nil {
		return "{}"
	}

	// Check if obj is identified as potentially being sensitive
	switch o := obj.(type) {
	case *corev1.ConfigMap:
		cloned := o.DeepCopy()
		return redactConfigMap(cloned)
	case *appsv1.Deployment:
		cloned := o.DeepCopy()
		return redactDeployment(cloned)
	case *networkingv1.Ingress:
		cloned := o.DeepCopy()
		return redactIngress(cloned)
	case *oauthv1.OAuthClient:
		cloned := o.DeepCopy()
		return redactOAuthClient(cloned)
	case *routev1.Route:
		cloned := o.DeepCopy()
		return redactRoute(cloned)
	default:
		return jsonString(o)
	}
}

// IsSSL Determine if deployment should be via SSL or not
// SSL should be switched on for OpenShift 4+
// For backward compatibility if there is no SSL Prop then default to true
func IsSSL(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec) bool {
	sslLogMsg := "Should deployment use SSL: "

	if apiSpec.IsOpenShift4 {
		sslLogMsg = fmt.Sprintf("%s true [Using OpenShift]\n", sslLogMsg)
		return true // Always on for OpenShift 4+
	}

	if hawtio.Spec.Auth.InternalSSL == nil {
		sslLogMsg = fmt.Sprintf("%s true [InternalSSL not defined]\n", sslLogMsg)
		return true // Should be switched on by default
	}

	log.V(DebugLogLevel).Info(fmt.Sprintf("%s %t [Value of InternalSSL in CR]\n", sslLogMsg, *hawtio.Spec.Auth.InternalSSL))
	return *hawtio.Spec.Auth.InternalSSL
}
