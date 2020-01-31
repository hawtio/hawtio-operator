package openshift

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	v1template "github.com/openshift/api/template/v1"

	"github.com/hawtio/hawtio-operator/pkg/openshift/util"
)

type TemplateProcessor struct {
	restClient *rest.RESTClient
}

func NewTemplateProcessor(inConfig *rest.Config) (*TemplateProcessor, error) {
	config := rest.CopyConfig(inConfig)
	config.GroupVersion = &schema.GroupVersion{
		Group:   "template.openshift.io",
		Version: "v1",
	}
	config.APIPath = "/apis"
	config.AcceptContentTypes = "application/json"
	config.ContentType = "application/json"

	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	restClient, err := rest.RESTClientFor(config)
	if err != nil {
		return nil, err
	}

	return &TemplateProcessor{
		restClient: restClient,
	}, nil
}

func (p *TemplateProcessor) Process(template *v1template.Template, namespace string, parameters map[string]string) ([]runtime.RawExtension, error) {
	fillParameters(template, parameters)

	resource, err := json.Marshal(template)
	if err != nil {
		return nil, err
	}

	result := p.restClient.
		Post().
		Namespace(namespace).
		Body(resource).
		Resource("processedtemplates").
		Do()

	if result.Error() == nil {
		data, err := result.Raw()
		if err != nil {
			return nil, err
		}

		templ, err := util.LoadKubernetesResource(data)
		if err != nil {
			return nil, err
		}

		if v1Temp, ok := templ.(*v1template.Template); ok {
			return v1Temp.Objects, nil
		}

		return nil, fmt.Errorf("wrong type returned by the server: %v", templ)
	}

	return nil, result.Error()
}

func fillParameters(tmpl *v1template.Template, params map[string]string) {
	for i, param := range tmpl.Parameters {
		if value, ok := params[param.Name]; ok {
			tmpl.Parameters[i].Value = value
		}
	}
}
