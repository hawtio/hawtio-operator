package resources

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"

	"github.com/stretchr/testify/assert"
)

func TestPropagateAnnotations(t *testing.T) {
	hawtio := &hawtiov1alpha1.Hawtio{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation1": "value1",
				"annotation2": "value2",
				"annotation3": "value3",
			},
		},
		Spec: hawtiov1alpha1.HawtioSpec{
			MetadataPropagation: hawtiov1alpha1.HawtioMetadataPropagation{
				Annotations: []string{"annotation2"},
			},
		},
	}
	annotations := map[string]string{
		"annotation1": "value0",
	}
	propagateAnnotations(hawtio, annotations)
	assert.Len(t, annotations, 2)
	assert.Equal(t, "value0", annotations["annotation1"])
	assert.Equal(t, "value2", annotations["annotation2"])
}

func TestPropagateLabels(t *testing.T) {
	hawtio := &hawtiov1alpha1.Hawtio{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"label1": "value1",
				"label2": "value2",
				"label3": "value3",
			},
		},
		Spec: hawtiov1alpha1.HawtioSpec{
			MetadataPropagation: hawtiov1alpha1.HawtioMetadataPropagation{
				Labels: []string{"label2"},
			},
		},
	}
	labels := map[string]string{
		"label1": "value0",
	}
	propagateLabels(hawtio, labels)
	assert.Len(t, labels, 2)
	assert.Equal(t, "value0", labels["label1"])
	assert.Equal(t, "value2", labels["label2"])
}
