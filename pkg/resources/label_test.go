package resources

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hawtiov1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1"

	"github.com/stretchr/testify/assert"
)

func TestPropagateAnnotations(t *testing.T) {
	hawtio := &hawtiov1.Hawtio{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation1":          "value1",
				"annotation2":          "value2",
				"annotation3":          "value3",
				"group.io/annotation1": "value4",
				"group.io/annotation2": "value5",
				"group.io/annotation3": "value6",
			},
		},
		Spec: hawtiov1.HawtioSpec{
			MetadataPropagation: hawtiov1.HawtioMetadataPropagation{
				Annotations: []string{
					"annotation1",
					"annotation2",
					"group.io/*",
				},
			},
		},
	}
	annotations := map[string]string{
		"annotation1": "value0",
	}
	propagateAnnotations(hawtio, annotations)
	assert.Len(t, annotations, 5)
	assert.Equal(t, "value0", annotations["annotation1"])
	assert.Equal(t, "value2", annotations["annotation2"])
	assert.Equal(t, "value4", annotations["group.io/annotation1"])
	assert.Equal(t, "value5", annotations["group.io/annotation2"])
	assert.Equal(t, "value6", annotations["group.io/annotation3"])
}

func TestPropagateLabels(t *testing.T) {
	hawtio := &hawtiov1.Hawtio{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"label1":          "value1",
				"label2":          "value2",
				"label3":          "value3",
				"group.io/label1": "value4",
				"group.io/label2": "value5",
				"group.io/label3": "value6",
			},
		},
		Spec: hawtiov1.HawtioSpec{
			MetadataPropagation: hawtiov1.HawtioMetadataPropagation{
				Labels: []string{
					"label1",
					"label2",
					"group.io/*",
				},
			},
		},
	}
	labels := map[string]string{
		"label1": "value0",
	}
	propagateLabels(hawtio, labels)
	assert.Len(t, labels, 5)
	assert.Equal(t, "value0", labels["label1"])
	assert.Equal(t, "value2", labels["label2"])
	assert.Equal(t, "value4", labels["group.io/label1"])
	assert.Equal(t, "value5", labels["group.io/label2"])
	assert.Equal(t, "value6", labels["group.io/label3"])
}
