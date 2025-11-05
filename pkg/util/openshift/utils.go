/**
 * Package openshift contains utility functions taken from
 * https://github.com/RHsyseng/operator-utils/blob/main/pkg
 *
 * Licensed under Apache 2.0
 */
package openshift

import (
	"errors"
	"strconv"

	consolev1 "github.com/openshift/api/console/v1"
	amv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func GetConsoleYAMLSample(res client.Object) (*consolev1.ConsoleYAMLSample, error) {
	annotations := res.GetAnnotations()
	snippetStr := annotations["consoleSnippet"]
	var snippet = false
	if tmp, err := strconv.ParseBool(snippetStr); err == nil {
		snippet = tmp
	}

	targetAPIVersion, _ := annotations["consoleTargetAPIVersion"]
	if targetAPIVersion == "" {
		targetAPIVersion = res.GetObjectKind().GroupVersionKind().GroupVersion().String()
	}

	targetKind := annotations["consoleTargetKind"]
	if targetKind == "" {
		targetKind = res.GetObjectKind().GroupVersionKind().Kind
	}

	defaultText := res.GetName() + "-yamlsample"
	title, _ := annotations["consoleTitle"]
	if title == "" {
		title = defaultText
	}
	desc, _ := annotations["consoleDesc"]
	if desc == "" {
		desc = defaultText
	}
	name, _ := annotations["consoleName"]
	if name == "" {
		name = defaultText
	}

	delete(annotations, "consoleSnippet")
	delete(annotations, "consoleTitle")
	delete(annotations, "consoleDesc")
	delete(annotations, "consoleName")
	delete(annotations, "consoleTargetAPIVersion")
	delete(annotations, "consoleTargetKind")

	data, err := yaml.Marshal(res)
	if err != nil {
		return nil, errors.New("failed to convert to yamlstr from KubernetesResource")
	}

	yamlSample := &consolev1.ConsoleYAMLSample{
		ObjectMeta: amv1.ObjectMeta{
			Name:      name,
			Namespace: "openshift-console",
		},
		Spec: consolev1.ConsoleYAMLSampleSpec{
			TargetResource: amv1.TypeMeta{
				APIVersion: targetAPIVersion,
				Kind:       targetKind,
			},
			Title:       consolev1.ConsoleYAMLSampleTitle(title),
			Description: consolev1.ConsoleYAMLSampleDescription(desc),
			YAML:        consolev1.ConsoleYAMLSampleYAML(string(data)),
			Snippet:     snippet,
		},
	}
	return yamlSample, nil
}
