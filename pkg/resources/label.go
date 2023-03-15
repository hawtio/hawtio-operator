package resources

import (
	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

const (
	labelAppKey      = "app"
	labelResourceKey = "deployment"
)

// Set labels in a map
func labelsForHawtio(name string) map[string]string {
	return map[string]string{
		labelAppKey:      "hawtio",
		labelResourceKey: name,
	}
}

func propagateAnnotations(hawtio *hawtiov1alpha1.Hawtio, annotations map[string]string) {
	for k, v := range hawtio.GetAnnotations() {
		// Only propagate specified annotations
		if !util.MatchPatterns(hawtio.Spec.MetadataPropagation.Annotations, k) {
			continue
		}
		// Not overwrite existing annotations
		if _, ok := annotations[k]; !ok {
			annotations[k] = v
		}
	}
}

func propagateLabels(hawtio *hawtiov1alpha1.Hawtio, labels map[string]string) {
	for k, v := range hawtio.GetLabels() {
		// Only propagate specified labels
		if !util.MatchPatterns(hawtio.Spec.MetadataPropagation.Labels, k) {
			continue
		}
		// Not overwrite existing labels
		if _, ok := labels[k]; !ok {
			labels[k] = v
		}
	}
}
