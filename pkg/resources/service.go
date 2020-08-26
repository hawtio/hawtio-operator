package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
)

//func NewServiceDefinitionForCR
func NewServiceDefinitionForCR(cr *hawtiov1alpha1.Hawtio) *corev1.Service {
	name := cr.Name

	port := corev1.ServicePort{
		Name:       name,
		Protocol:   "TCP",
		Port:       443,
		TargetPort: intstr.FromString("nginx"),
	}
	var ports []corev1.ServicePort
	ports = append(ports, port)

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"service.alpha.openshift.io/serving-cert-secret-name": name + "-tls-serving",
				"hawtio.hawt.io/hawtioversion":                        cr.GetResourceVersion(),
			},
			Labels: map[string]string{"app": "hawtio"},
			Name:   name,
		},
		Spec: corev1.ServiceSpec{
			Ports:                    ports,
			Selector:                 labelsForHawtio(name),
			SessionAffinity:          "None",
			PublishNotReadyAddresses: true,
		},
	}

	return svc
}
