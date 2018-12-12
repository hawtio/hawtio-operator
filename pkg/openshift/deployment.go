package openshift

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	appsv1 "github.com/openshift/api/apps/v1"

	"github.com/hawtio/hawtio-operator/pkg/openshift/util"
)

type DeploymentClient struct {
	restClient *rest.RESTClient
}

func NewDeploymentClient(inConfig *rest.Config) (*DeploymentClient, error) {
	config := rest.CopyConfig(inConfig)
	config.GroupVersion = &schema.GroupVersion{
		Group:   "apps.openshift.io",
		Version: "v1",
	}
	config.APIPath = "/apis"
	config.AcceptContentTypes = "application/json"
	config.ContentType = "application/json"

	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: scheme.Codecs}
	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	restClient, err := rest.RESTClientFor(config)
	if err != nil {
		return nil, err
	}

	return &DeploymentClient{
		restClient: restClient,
	}, nil
}

func (p *DeploymentClient) Deploy(request *appsv1.DeploymentRequest, namespace string) (*appsv1.DeploymentConfig, error) {
	resource, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	req := p.restClient.
		Post().
		Namespace(namespace).
		Body(resource).
		Resource("deploymentconfigs").
		SubResource(request.Name).
		Suffix("instantiate")

	result := req.Do()

	if result.Error() == nil {
		data, err := result.Raw()
		if err != nil {
			return nil, err
		}

		res, err := util.LoadKubernetesResource(data)
		if err != nil {
			return nil, err
		}

		if res, ok := res.(*appsv1.DeploymentConfig); ok {
			return res, nil
		}

		return nil, fmt.Errorf("Wrong type returned by the server: %v", res)
	}

	return nil, result.Error()
}
