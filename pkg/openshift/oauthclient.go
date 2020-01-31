package openshift

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	oauthv1 "github.com/openshift/api/oauth/v1"

	"github.com/hawtio/hawtio-operator/pkg/openshift/util"
)

type OAuthClientClient struct {
	restClient *rest.RESTClient
}

func NewOAuthClientClient(inConfig *rest.Config) (*OAuthClientClient, error) {
	config := rest.CopyConfig(inConfig)
	config.GroupVersion = &schema.GroupVersion{
		Group:   "oauth.openshift.io",
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

	return &OAuthClientClient{
		restClient: restClient,
	}, nil
}

func (p *OAuthClientClient) Get(name string) (*oauthv1.OAuthClient, error) {
	req := p.restClient.
		Get().
		Resource("oauthclients").
		SubResource(name)

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

		if res, ok := res.(*oauthv1.OAuthClient); ok {
			return res, nil
		}

		return nil, fmt.Errorf("wrong type returned by the server: %v", res)
	}

	return nil, result.Error()
}
