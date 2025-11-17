package openshift

import (
	"context"
	"io/fs"

	"github.com/hawtio/hawtio-operator/deploy"
	"github.com/hawtio/hawtio-operator/pkg/util/kubernetes"
	"github.com/hawtio/hawtio-operator/pkg/util/openshift"
	"sigs.k8s.io/yaml"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
)

var log = logf.Log.WithName("openshift-webconsole")

func ConsoleYAMLSampleExists() error {
	gvk := schema.GroupVersionKind{Group: "console.openshift.io", Version: "v1", Kind: "ConsoleYAMLSample"}
	return kubernetes.CustomResourceDefinitionExists(gvk)
}

func CreateConsoleYAMLSamples(ctx context.Context, c client.Client, productName string) {
	log.Info("Loading CR YAML samples.")

	// Read the embedded directory. The path inside the embed.FS is relative,
	// so we use "deploy/crs" to access the contents.
	files, err := fs.ReadDir(deploy.CRS, "crs")
	if err != nil || len(files) == 0 {
		log.Error(err, "CR YAML folder is empty or could not be read. It is not loaded.")
		return
	}

	// Loop through the files using the standard 'fs.DirEntry' type
	for _, file := range files {
		filename := file.Name()
		if filename == "kustomization.yaml" {
			continue
		}

		// Read the file content using fs.ReadFile. The path must include the directory.
		filePath := "crs/" + filename
		yamlBytes, err := fs.ReadFile(deploy.CRS, filePath)
		if err != nil {
			log.Info("yaml", " name: ", filename, " not created: ", err.Error())
			continue
		}

		// The rest of your logic remains exactly the same!
		hawtio := hawtiov2.NewHawtio()
		err = yaml.Unmarshal(yamlBytes, &hawtio)
		if err != nil {
			log.Info("yaml", " name: ", filename, " not created: ", err.Error())
			continue
		}
		if productName != "" {
			hawtio.ObjectMeta.Name = productName
		}
		yamlSample, err := openshift.GetConsoleYAMLSample(hawtio)
		if err != nil {
			log.Info("yaml", " name: ", filename, " not created: ", err.Error())
			continue
		}
		err = c.Create(ctx, yamlSample)
		if err != nil {
			if !apierrors.IsAlreadyExists(err) {
				log.Info("yaml", " name: ", filename, " not created: ", err.Error())
			}
			continue
		}
		log.Info("yaml created", "name", filename)
	}
}
