package kubernetes

import (
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
