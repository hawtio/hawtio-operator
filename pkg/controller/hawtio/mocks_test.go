package hawtio

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	hawtiov1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1"
)

var (
	hawtio = hawtiov1.Hawtio{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hawtio-online-test",
			Namespace: "hawtio-online-ns",
			Labels: map[string]string{
				"app": "hawtio",
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Hawtio",
			APIVersion: "hawt.io/v1",
		},
		Spec: hawtiov1.HawtioSpec{
			Type:          hawtiov1.NamespaceHawtioDeploymentType,
			Version:       "latest",
			RouteHostName: "hawtio.cluster",
			Config: hawtiov1.HawtioConfig{
				Online: hawtiov1.HawtioOnline{
					ProjectSelector: "!openshift.io/run-level,!openshift.io/cluster-monitoring",
				},
			},
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("100Mi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
			},
		},
	}
)
