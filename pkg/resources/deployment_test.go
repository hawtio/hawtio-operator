package resources

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetServingCertificateMountPath(t *testing.T) {
	// version 'latest' should pass
	path, err := getServingCertificateMountPath("latest", "< 1.2.0")
	assert.NoError(t, err)
	assert.Equal(t, serviceSigningSecretVolumeMountPath, path)

	// a standard version should pass
	path, err = getServingCertificateMountPath("1.0.0", "< 1.2.0")
	assert.NoError(t, err)
	assert.Equal(t, serviceSigningSecretVolumeMountPathLegacy, path)

	// any arbitrary tag name as a version should also pass
	path, err = getServingCertificateMountPath("test", "< 1.2.0")
	assert.NoError(t, err)
	assert.Equal(t, serviceSigningSecretVolumeMountPath, path)
}
