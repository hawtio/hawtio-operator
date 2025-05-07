package resources

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/go-logr/logr"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

const (
	PlainServicePort = 80
	SSLServicePort   = 443
)

func NewService(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec, log logr.Logger) *corev1.Service {
	log.V(util.DebugLogLevel).Info("Reconciling service")

	name := hawtio.Name

	annotations := map[string]string{
		"service.beta.openshift.io/serving-cert-secret-name": name + "-tls-serving",
	}
	PropagateAnnotations(hawtio, annotations, log)

	labels := map[string]string{
		LabelAppKey: "hawtio",
	}
	PropagateLabels(hawtio, labels, log)

	servicePort := PlainServicePort
	if util.IsSSL(hawtio, apiSpec) {
		servicePort = SSLServicePort
	}

	service := &corev1.Service{
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
					Port:       int32(servicePort),
					TargetPort: intstr.FromString(containerPortName),
				},
			},
			Selector:                 LabelsForHawtio(name),
			SessionAffinity:          "None",
			PublishNotReadyAddresses: true,
		},
	}

	log.V(util.DebugLogLevel).Info(fmt.Sprintf("New service %s", util.JSONToString(service)))
	return service
}
