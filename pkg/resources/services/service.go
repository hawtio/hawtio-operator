package services

import (
	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	"github.com/hawtio/hawtio-operator/pkg/util/selectors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("package services")

//func NewServiceDefinitionForCR
func NewServiceDefinitionForCR(cr *hawtiov1alpha1.Hawtio) *corev1.Service {

	reqLogger := log.WithName(cr.Name)
	reqLogger.Info("Creating new Service Definition for custom resource")
	applicationName := cr.Name

	port := corev1.ServicePort{
		Name:       applicationName,
		Protocol:   "TCP",
		Port:       443,
		TargetPort: intstr.FromString("nginx"),
	}
	ports := []corev1.ServicePort{}
	ports = append(ports, port)

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"service.alpha.openshift.io/serving-cert-secret-name": applicationName + "-tls-serving",
				"hawtio.hawt.io/hawtioversion":                        cr.GetResourceVersion(),
			},
			Labels: map[string]string{"app": "hawtio"},
			Name:   applicationName,
		},
		Spec: corev1.ServiceSpec{
			Ports:                    ports,
			Selector:                 selectors.LabelsForHawtio(applicationName),
			SessionAffinity:          "None",
			PublishNotReadyAddresses: true,
		},
	}

	return svc
}
