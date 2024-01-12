package kubernetes

import (
	"strconv"

	hawtiov1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/hawtio/hawtio-operator/pkg/resources"
)

// NewIngress create a new Ingress resource
func NewIngress(hawtio *hawtiov1.Hawtio, servingSecret *corev1.Secret) *networkingv1.Ingress {
	name := hawtio.Name

	annotations := map[string]string{}
	annotations["nginx.ingress.kubernetes.io/backend-protocol"] = "HTTPS"
	annotations["nginx.ingress.kubernetes.io/force-ssl-redirect"] = "true"
	annotations["nginx.ingress.kubernetes.io/rewrite-target"] = "/$1"

	resources.PropagateAnnotations(hawtio, annotations)

	labels := map[string]string{
		resources.LabelAppKey: "hawtio",
	}
	resources.PropagateLabels(hawtio, labels)

	ingressTLS := networkingv1.IngressTLS{}
	if servingSecret != nil {
		ingressTLS.SecretName = servingSecret.Name
	}

	pathPrefix := networkingv1.PathTypePrefix

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: annotations,
			Labels:      labels,
			Name:        name,
		},
		Spec: networkingv1.IngressSpec{
			TLS: []networkingv1.IngressTLS{
				ingressTLS,
			},
			Rules: []networkingv1.IngressRule{
				{
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{{
								Path:     "/(.*)",
								PathType: &pathPrefix,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: hawtio.Name,
										Port: networkingv1.ServiceBackendPort{
											Number: 443,
										},
									},
								},
							}},
						},
					},
				},
			},
		},
	}

	return ingress
}

// GetIngressURL determines the full URL of the given ingress
func GetIngressURL(ingress *networkingv1.Ingress) string {
	var scheme string
	if len(ingress.Spec.TLS) > 0 {
		scheme = "https"
	} else {
		scheme = "http"
	}

	host, port := getIngressHostAndPort(ingress)
	path := getIngressPath(ingress)

	url := scheme + "://" + host
	if len(port) > 0 {
		url = url + ":" + port
	}

	return url + path
}

func getIngressHostAndPort(ingress *networkingv1.Ingress) (string, string) {
	ingressStatuses := ingress.Status.LoadBalancer.Ingress
	if len(ingressStatuses) == 0 {
		for _, ingressRule := range ingress.Spec.Rules {
			if len(ingressRule.Host) > 0 {
				return ingressRule.Host, ""
			}
		}

		return "*", "" // host must be a wildcard
	}

	// get host or ip of the first ingress status available
	var host string
	port := ""
	for _, ingressStatus := range ingress.Status.LoadBalancer.Ingress {
		if len(ingressStatus.Hostname) > 0 {
			host = ingressStatus.Hostname
		} else if len(ingressStatus.IP) > 0 {
			host = ingressStatus.IP
		}

		if len(host) > 0 {
			for _, statusPort := range ingressStatus.Ports {
				port = strconv.FormatInt(int64(statusPort.Port), 10)
				continue // get the first port
			}
		}
	}

	if len(host) == 0 {
		return "*", port
	}

	return host, port
}

func getIngressPath(ingress *networkingv1.Ingress) string {
	for _, ingressRule := range ingress.Spec.Rules {
		if ingressRule.IngressRuleValue.HTTP != nil {
			for _, httpPath := range ingressRule.IngressRuleValue.HTTP.Paths {
				return httpPath.Path
			}
		}
	}

	return "/"
}
