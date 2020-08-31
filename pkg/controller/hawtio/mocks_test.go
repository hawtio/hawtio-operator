package hawtio

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
)

var (
	HawtioInstance = hawtiov1alpha1.Hawtio{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hawtio-online-test",
			Namespace: "hawtio-online-ns",
			Labels: map[string]string{
				"app": "hawtio",
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Hawtio",
			APIVersion: "hawt.io/v1alpha1",
		},
		Spec: hawtiov1alpha1.HawtioSpec{
			Type:          "namespace",
			Version:       "latest",
			RouteHostName: "hawtio.cluster",
		},
	}
)
