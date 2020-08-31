package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func newPodTemplateSpec(namespacedName types.NamespacedName, labels map[string]string, configMapVersion string) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
			Labels:    labels,
			// Used to trigger a rollout deployment if config changed,
			// similarly to `kubectl rollout restart`
			Annotations: map[string]string{
				configVersionAnnotation: configMapVersion,
			},
		},
	}
}
