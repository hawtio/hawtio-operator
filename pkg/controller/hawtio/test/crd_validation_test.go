package test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"

	"github.com/RHsyseng/operator-utils/pkg/validation"

	"github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
)

func TestSampleCustomResources(t *testing.T) {
	schema := getSchema(t)
	assert.NotNil(t, schema)

	filePath := getCRFile(t, "../../../../deploy/crs")
	bytes, err := ioutil.ReadFile(filePath)
	assert.NoError(t, err, "Error reading CR yaml %v", filePath)

	var input map[string]interface{}
	assert.NoError(t, yaml.Unmarshal(bytes, &input))
	assert.NoError(t, schema.Validate(input), "File %v does not validate against the CRD schema", filePath)
}

func TestTrialEnvMinimum(t *testing.T) {
	var inputYaml = `
apiVersion: hawt.io/v1alpha1
kind: Hawtio
metadata:
  name: trial
spec:
  type: namespace
  replicas: 1
  version: latest
`
	var input map[string]interface{}
	assert.NoError(t, yaml.Unmarshal([]byte(inputYaml), &input))

	schema := getSchema(t)
	assert.NoError(t, schema.Validate(input))
}

// Requires openAPIV3Schema in CRD for function to work properly
func TestCompleteCRD(t *testing.T) {
	schema := getSchema(t)
	missingEntries := schema.GetMissingEntries(&v1alpha1.Hawtio{})
	assert.Emptyf(t, missingEntries, "Missing or incorrect schema validation: %v", missingEntries)
}

func getCRFile(t *testing.T, dir string) string {
	var file string
	err := filepath.Walk(dir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				file = path
			}
			return nil
		})
	assert.NoError(t, err, "Error finding CR yaml %v", file)
	return file
}

func getSchema(t *testing.T) validation.Schema {
	crdFile := "../../../../deploy/crd/hawtio_v1alpha1_hawtio_crd.yaml"
	bytes, err := ioutil.ReadFile(crdFile)
	assert.NoError(t, err, "Error reading CRD yaml %v", crdFile)
	schema, err := validation.New(bytes)
	assert.NoError(t, err)
	return schema
}
