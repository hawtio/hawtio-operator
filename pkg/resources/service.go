package resources

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/go-logr/logr"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	common "github.com/hawtio/hawtio-operator/pkg/resources/common"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

func NewDefaultService(hawtio *hawtiov2.Hawtio) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hawtio.Name,
			Namespace: hawtio.Namespace,
		},
	}
}

func NewService(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec, log logr.Logger) *corev1.Service {
	log.V(util.DebugLogLevel).Info("Reconciling service")

	name := hawtio.Name

	annotations := map[string]string{
		"service.beta.openshift.io/serving-cert-secret-name": name + "-tls-serving",
	}
	common.PropagateAnnotations(hawtio, annotations, log)

	labels := common.LabelsForHawtio(hawtio.Name)
	common.PropagateLabels(hawtio, labels, log)

	servicePort := common.PlainServicePort
	if util.IsSSL(hawtio, apiSpec) {
		servicePort = common.SSLServicePort
	}

	service := NewDefaultService(hawtio)
	service.SetLabels(labels)
	service.SetAnnotations(annotations)
	service.Spec = corev1.ServiceSpec{
		Ports: []corev1.ServicePort{
			{
				Name:       name,
				Protocol:   "TCP",
				Port:       int32(servicePort),
				TargetPort: intstr.FromString(common.ContainerPortName),
			},
		},
		Selector:                 common.LabelsForHawtio(name),
		SessionAffinity:          "None",
		PublishNotReadyAddresses: true,
	}

	log.V(util.DebugLogLevel).Info(fmt.Sprintf("New service %s", util.ToJSONString(service)))
	return service
}
