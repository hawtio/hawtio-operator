package resources

import (
	"fmt"

	"github.com/go-logr/logr"
	commonapi "github.com/hawtio/hawtio-operator/pkg/apis/common"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

const (
	LabelAppKey      = "app"
	LabelAppValue    = "hawtio"
	labelResourceKey = "deployment"
)

// LabelsForHawtio Set labels in a map
func LabelsForHawtio(name string) map[string]string {
	return map[string]string{
		LabelAppKey:      LabelAppValue,
		labelResourceKey: name,
	}
}

// PropagateAnnotations propagate annotations from hawtio CR
func PropagateAnnotations(hawtio commonapi.HawtioGeneral, annotations map[string]string, log logr.Logger) {
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Propagating Annotations %s", util.ToJSONString(annotations)))

	for k, v := range hawtio.GetAnnotations() {
		// Only propagate specified annotations
		if !util.MatchPatterns(hawtio.GetMetadataInfo().AnnotationPatterns, k) {
			continue
		}
		// Not overwrite existing annotations
		if _, ok := annotations[k]; !ok {
			annotations[k] = v
		}
	}
}

// PropagateLabels propagate labels from hawtio CR
func PropagateLabels(hawtio commonapi.HawtioGeneral, labels map[string]string, log logr.Logger) {
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Propagating Labels %s", util.ToJSONString(labels)))

	for k, v := range hawtio.GetLabels() {
		// Only propagate specified labels
		if !util.MatchPatterns(hawtio.GetMetadataInfo().LabelPatterns, k) {
			continue
		}
		// Not overwrite existing labels
		if _, ok := labels[k]; !ok {
			labels[k] = v
		}
	}
}
