package kubernetes

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/hawtio/hawtio-operator/pkg/resources"

	"github.com/stretchr/testify/assert"
)

func TestGetIngressURL(t *testing.T) {
	annotations := map[string]string{}
	annotations["nginx.ingress.kubernetes.io/backend-protocol"] = "HTTPS"
	annotations["nginx.ingress.kubernetes.io/force-ssl-redirect"] = "true"
	annotations["nginx.ingress.kubernetes.io/rewrite-target"] = "/$1"

	labels := map[string]string{
		resources.LabelAppKey: "hawtio",
	}
	pathPrefix := networkingv1.PathTypePrefix
	name := "hawtio-online"

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: annotations,
			Labels:      labels,
			Name:        name,
		},
		Spec: networkingv1.IngressSpec{
			TLS: []networkingv1.IngressTLS{
				{
					SecretName: "some-tls-secret",
				},
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
										Name: name,
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
		Status: networkingv1.IngressStatus{
			LoadBalancer: networkingv1.IngressLoadBalancerStatus{
				Ingress: []networkingv1.IngressLoadBalancerIngress{
					{
						IP: "192.168.99.9",
					},
				},
			},
		},
	}

	url := GetIngressURL(ingress)
	assert.Equal(t, "https://192.168.99.9/(.*)", url)
}
