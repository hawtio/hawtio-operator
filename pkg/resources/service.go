package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	hawtiov1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1"
)

func NewService(hawtio *hawtiov1.Hawtio) *corev1.Service {
	name := hawtio.Name

	annotations := map[string]string{
		"service.beta.openshift.io/serving-cert-secret-name": name + "-tls-serving",
	}
	propagateAnnotations(hawtio, annotations)

	labels := map[string]string{
		labelAppKey: "hawtio",
	}
	propagateLabels(hawtio, labels)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: annotations,
			Labels:      labels,
			Name:        name,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       name,
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromString(containerPortName),
				},
			},
			Selector:                 labelsForHawtio(name),
			SessionAffinity:          "None",
			PublishNotReadyAddresses: true,
		},
	}
}
