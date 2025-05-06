package hawtio

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
)

func initHawtio(includeSSL int) *hawtiov2.Hawtio {
	hawtio := hawtiov2.NewHawtio()

	hawtio.ObjectMeta = metav1.ObjectMeta{
		Name:      hawtioCRName,
		Namespace: "hawtio-online-ns",
		Labels: map[string]string{
			"app": "hawtio",
		},
	}

	hawtio.TypeMeta = metav1.TypeMeta{
		Kind:       "Hawtio",
		APIVersion: "hawt.io/v1",
	}

	hawtio.Spec = hawtiov2.HawtioSpec{
		Type:          hawtiov2.NamespaceHawtioDeploymentType,
		RouteHostName: "hawtio.cluster",
		Config: hawtiov2.HawtioConfig{
			Online: hawtiov2.HawtioOnline{
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
	}

	if includeSSL > -1 {
		internalSSL := includeSSL > 0

		hawtio.Spec.Auth = hawtiov2.HawtioAuth{
			InternalSSL: &internalSSL,
		}
	}

	return hawtio
}

var hawtioCRName = "hawtio-online-test"

// SSL never specified so property is null
var defaultHawtio = initHawtio(-1)

// SSL specified as false
var plainHawtio = initHawtio(0)

// SSL specified as true
var sslHawtio = initHawtio(1)
