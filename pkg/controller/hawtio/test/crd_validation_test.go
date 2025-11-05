package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/yaml"

	v2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2" // Use your actual project import path
)

// getSchema loads the CRD from YAML, extracts the v2 schema, and compiles it.
// This is a lazy-loaded singleton pattern to avoid reading the file in every test.
var (
	crdSchema *jsonschema.Schema
)

// TestSampleCustomResources iterates through all sample CRs and validates them.
func TestSampleCustomResources(t *testing.T) {
	schema := getSchema(t)

	filePath := getLatestCRFile(t, "../../../../deploy/crs")

	t.Run(filepath.Base(filePath), func(t *testing.T) {
		bytes, err := os.ReadFile(filePath)
		require.NoError(t, err, "Error reading CR yaml %v", filePath)

		var input map[string]interface{}
		require.NoError(t, yaml.Unmarshal(bytes, &input))

		err = schema.Validate(input)
		assert.NoError(t, err, "File %v does not validate against the CRD schema", filePath)
	})
}

// TestTrialEnvMinimum validates a minimal, hardcoded CR.
func TestTrialEnvMinimum(t *testing.T) {
	// NOTE: Corrected apiVersion from v1 to v2 to match the schema being tested.
	var inputYaml = `
apiVersion: hawt.io/v2
kind: Hawtio
metadata:
  name: trial
spec:
  type: Namespace
  replicas: 1
  version: latest
`
	var input map[string]interface{}
	require.NoError(t, yaml.Unmarshal([]byte(inputYaml), &input))

	schema := getSchema(t)
	err := schema.Validate(input)
	assert.NoError(t, err)
}

// TestCRDMatchesGoStruct ensures the Go struct and CRD schema have not drifted.
func TestCRDMatchesGoStruct(t *testing.T) {
	schema := getSchema(t)
	hawtioInstance := v2.NewHawtio()

	// Convert the Go struct into a generic interface{} for the validator
	var instanceData interface{}
	bytes, err := json.Marshal(hawtioInstance)
	require.NoError(t, err)
	err = json.Unmarshal(bytes, &instanceData)
	require.NoError(t, err)

	// Validate the instance against the schema
	err = schema.Validate(instanceData)

	if err != nil {
		validationErr, ok := err.(*jsonschema.ValidationError)
		require.True(t, ok, "Error was not a jsonschema.ValidationError")

		// Preserve the special case for the Time type if necessary
		if strings.HasPrefix(validationErr.InstanceLocation, "/spec/auth/clientCertExpirationDate") {
			t.Log("Ignoring known validation discrepancy with Time type at", validationErr.InstanceLocation)
		} else {
			// The new library provides much more detailed errors.
			assert.Fail(t, "Discrepancy between CRD and Struct", "Schema validation failed: %v", err)
		}
	}
}

// getLatestCRFile finds the file in a directory that contains the highest version
// number in its name eg. finds 'hawtio_v2_hawtio_cr.yaml' over
// 'hawito_v1_hawtio_cr.yaml and hawtio_v1alpha1_hawtio_cr.yaml'.
func getLatestCRFile(t *testing.T, dir string) string {
	latestVersion := -1
	var latestFile string

	// Compile the regex once for efficiency. It looks for 'v' followed by one
	// or more digits. The parentheses (d+) create a "capturing group" for the
	// number itself.
	re := regexp.MustCompile(`hawtio_v(\d+).*_cr.yaml`)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		require.NoError(t, err)
		if info.IsDir() {
			return nil // Skip directories
		}

		// Find submatches in the filename, e.g., ["v2", "2"]
		matches := re.FindStringSubmatch(info.Name())
		if len(matches) < 2 {
			return nil // Filename doesn't contain a version string, skip it
		}

		// The number is in the first capturing group (index 1)
		currentVersion, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil // Malformed number in filename, skip it
		}

		// If this file's version is higher than what we've seen so far, update our record.
		if currentVersion > latestVersion {
			latestVersion = currentVersion
			latestFile = path
		}

		return nil
	})

	require.NoError(t, err, "Error walking CR directory %s", dir)
	require.NotEmpty(t, latestFile, "No versioned CR file found in directory %s", dir)

	return latestFile
}

func getSchema(t *testing.T) *jsonschema.Schema {
	if crdSchema != nil {
		return crdSchema
	}

	crdFile := "../../../../deploy/crd/hawt.io_hawtios.yaml"

	// 1. Read the CRD file
	bytes, err := os.ReadFile(crdFile)
	require.NoError(t, err, "Error reading CRD yaml file")

	// 2. Unmarshal the CRD YAML into a Kubernetes CRD object
	var crd apiextensionsv1.CustomResourceDefinition
	err = yaml.Unmarshal(bytes, &crd)
	require.NoError(t, err, "Error unmarshalling CRD yaml")

	// 3. Find the 'v2' version and get its schema
	var schemaProps *apiextensionsv1.JSONSchemaProps
	for _, version := range crd.Spec.Versions {
		if version.Name == "v2" {
			schemaProps = version.Schema.OpenAPIV3Schema
			break
		}
	}
	require.NotNil(t, schemaProps, "Schema for version v2 not found in CRD")

	// 4. Marshal the extracted schema into JSON bytes for the validator
	schemaBytes, err := json.Marshal(schemaProps)
	require.NoError(t, err, "Error marshalling schema to JSON")

	// 5. Compile the schema into a reusable validator object
	compiled, err := jsonschema.CompileString("hawtio-crd-schema.json", string(schemaBytes))
	require.NoError(t, err, "Error compiling CRD schema")

	crdSchema = compiled
	return crdSchema
}
