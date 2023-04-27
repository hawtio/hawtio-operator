package resources

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAnnotations(t *testing.T) {
	sa, err := NewServiceAccountAsOauthClient("hawtio-online", []string{"one", "two", "three"})
	assert.NoError(t, err)
	assert.NotEmpty(t, sa.Annotations["serviceaccounts.openshift.io/oauth-redirectreference.hawtio-online"])
	assert.NotEmpty(t, sa.Annotations["serviceaccounts.openshift.io/oauth-redirectreference.one"])
	assert.NotEmpty(t, sa.Annotations["serviceaccounts.openshift.io/oauth-redirectreference.two"])
	assert.NotEmpty(t, sa.Annotations["serviceaccounts.openshift.io/oauth-redirectreference.three"])

	sa, err = NewServiceAccountAsOauthClient("hawtio-online", []string{})
	assert.NoError(t, err)
	assert.NotEmpty(t, sa.Annotations["serviceaccounts.openshift.io/oauth-redirectreference.hawtio-online"])

}
