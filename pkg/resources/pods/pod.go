package pods

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func MakePodTemplateSpec(namespacedName types.NamespacedName, labels map[string]string) corev1.PodTemplateSpec {
	pts := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
			Labels:    labels,
		},
	}
	return pts
}
