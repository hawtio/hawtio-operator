package gateway

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/go-logr/logr"

	hawtiopluginv1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtioplugin/v1"
	"github.com/hawtio/hawtio-operator/pkg/cfg"
	commonres "github.com/hawtio/hawtio-operator/pkg/resources/common"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

const (
	serviceName = "hawtio-online-console-plugin-gateway"
)

func NewDefaultService(namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
		},
	}
}

func NewService(hawtioPlugin *hawtiopluginv1.HawtioPlugin, namespace string, config cfg.DeploymentConfiguration, log logr.Logger) *corev1.Service {
	log.V(util.DebugLogLevel).Info("Reconciling service")

	/*
	 * metadata:
	 *	annotations:
	 *		service.beta.openshift.io/serving-cert-secret-name: hawtio-online-console-plugin-gateway-serving
	 */
	annotations := map[string]string{}
	if config.ServingCertSecretName != nil {
		annotations["service.beta.openshift.io/serving-cert-secret-name"] = *config.ServingCertSecretName
	}
	commonres.PropagateAnnotations(hawtioPlugin, annotations, log)

	/*
	 * labels:
	 *   app: hawtio
	 * 	 deployment: hawtio-online-console-plugin-gateway
	 *   app.kubernetes.io/component: hawtio-online-console-plugin-gateway
	 *   app.kubernetes.io/instance: hawtio-online-console-plugin-gateway
	 *   app.kubernetes.io/part-of: hawtio-online-console-plugin
	 */
	labels := commonres.LabelsForHawtio(gatewayName)
	labels["app.kubernetes.io/component"] = gatewayName
	labels["app.kubernetes.io/instance"] = gatewayName
	labels["app.kubernetes.io/part-of"] = commonres.LabelAppValue
	commonres.PropagateLabels(hawtioPlugin, labels, log)

	/*
	 * spec:
	 *   ports:
	 *   - name: 8443-tcp
	 *     protocol: TCP
	 *     port: 8443
	 *     targetPort: express
	 * selector:
	 *   app: hawtio-online-console-plugin-gateway
	 * type: ClusterIP
	 * sessionAffinity: None
	 */
	service := NewDefaultService(namespace)
	service.SetLabels(labels)
	service.SetAnnotations(annotations)
	service.Spec = corev1.ServiceSpec{
		Ports: []corev1.ServicePort{
			{
				Name:       fmt.Sprintf("%d-tcp", deploymentSSLPort),
				Protocol:   "TCP",
				Port:       int32(deploymentSSLPort),
				TargetPort: intstr.FromString(commonres.ContainerPortName),
			},
		},
		Selector: map[string]string{
			commonres.LabelAppKey: serviceName,
		},
		SessionAffinity:          "None",
		PublishNotReadyAddresses: true,
	}

	log.V(util.DebugLogLevel).Info(fmt.Sprintf("New service %s", util.ToJSONString(service)))
	return service
}
