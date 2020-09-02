package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func NewService(name string) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"service.alpha.openshift.io/serving-cert-secret-name": name + "-tls-serving",
			},
			Labels: map[string]string{"app": "hawtio"},
			Name:   name,
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
