package resources

import (
	"fmt"

	"github.com/go-logr/logr"
	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

const (
	LabelAppKey      = "app"
	labelResourceKey = "deployment"
)

// LabelsForHawtio Set labels in a map
func LabelsForHawtio(name string) map[string]string {
	return map[string]string{
		LabelAppKey:      "hawtio",
		labelResourceKey: name,
	}
}

// PropagateAnnotations propagate annotations from hawtio CR
func PropagateAnnotations(hawtio *hawtiov2.Hawtio, annotations map[string]string, log logr.Logger) {
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Propagating Annotations %s", util.JSONToString(annotations)))

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

// PropagateLabels propagate labels from hawtio CR
func PropagateLabels(hawtio *hawtiov2.Hawtio, labels map[string]string, log logr.Logger) {
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Propagating Labels %s", util.JSONToString(labels)))

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
