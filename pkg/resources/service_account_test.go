package resources

import (
	"testing"

	"github.com/go-logr/logr"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnnotations(t *testing.T) {
	hawtio := &hawtiov2.Hawtio{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hawtio-online",
			Namespace: "hawtio",
		},
		Spec: hawtiov2.HawtioSpec{
			ExternalRoutes: []string{
				"one", "two", "three",
			},
			Type: hawtiov2.NamespaceHawtioDeploymentType,
		},
	}
	log := logr.Discard()

	sa, err := NewServiceAccount(hawtio, log)
	assert.NoError(t, err)
	assert.NotEmpty(t, sa.Annotations["serviceaccounts.openshift.io/oauth-redirectreference.hawtio-online"])
	assert.NotEmpty(t, sa.Annotations["serviceaccounts.openshift.io/oauth-redirectreference.one"])
	assert.NotEmpty(t, sa.Annotations["serviceaccounts.openshift.io/oauth-redirectreference.two"])
	assert.NotEmpty(t, sa.Annotations["serviceaccounts.openshift.io/oauth-redirectreference.three"])

	hawtio.Spec.ExternalRoutes = []string{}
	sa, err = NewServiceAccount(hawtio, log)
	assert.NoError(t, err)
	assert.NotEmpty(t, sa.Annotations["serviceaccounts.openshift.io/oauth-redirectreference.hawtio-online"])

}
