package openshift

//go:generate go run ./.packr/packr.go

import (
	"context"

	"github.com/RHsyseng/operator-utils/pkg/logs"
	"github.com/RHsyseng/operator-utils/pkg/utils/kubernetes"
	"github.com/RHsyseng/operator-utils/pkg/utils/openshift"
	"github.com/ghodss/yaml"
	"github.com/gobuffalo/packr/v2"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
)

var log = logs.GetLogger("openshift-webconsole")

func ConsoleYAMLSampleExists() error {
	gvk := schema.GroupVersionKind{Group: "console.openshift.io", Version: "v1", Kind: "ConsoleYAMLSample"}
	return kubernetes.CustomResourceDefinitionExists(gvk)
}

func CreateConsoleYAMLSamples(c client.Client, productName string) {
	log.Info("Loading CR YAML samples.")
	box := packr.New("cryamlsamples", "../../deploy/crs")
	if box.List() == nil {
		log.Error("CR YAML folder is empty. It is not loaded.")
		return
	}
	for _, filename := range box.List() {
		yamlStr, err := box.FindString(filename)
		if err != nil {
			log.Info("yaml", " name: ", filename, " not created:  ", err.Error())
			continue
		}
		hawtio := hawtiov1alpha1.Hawtio{}
		err = yaml.Unmarshal([]byte(yamlStr), &hawtio)
		if err != nil {
			log.Info("yaml", " name: ", filename, " not created:  ", err.Error())
			continue
		}
		if productName != "" {
			hawtio.ObjectMeta.Name = productName
		}
		yamlSample, err := openshift.GetConsoleYAMLSample(&hawtio)
		if err != nil {
			log.Info("yaml", " name: ", filename, " not created:  ", err.Error())
			continue
		}
		err = c.Create(context.TODO(), yamlSample)
		if err != nil {
			if !apierrors.IsAlreadyExists(err) {
				log.Info("yaml", " name: ", filename, " not created:+", err.Error())
			}
			continue
		}
		log.Info("yaml", " name: ", filename, " Created.")
	}
}
