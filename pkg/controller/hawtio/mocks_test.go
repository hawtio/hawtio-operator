package hawtio

import (
	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	//corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//"k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
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
			Type:     "namespace",
			Replicas: 1,
			Version:  "latest",
		},
	}
)
