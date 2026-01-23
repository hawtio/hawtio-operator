package clients

import (
	"k8s.io/client-go/rest"

	configclient "github.com/openshift/client-go/config/clientset/versioned"
	oauthclient "github.com/openshift/client-go/oauth/clientset/versioned"
	kclient "k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

// ClientTools aggregates the typed clients required by the operator.
type ClientTools struct {
	CoreClient   corev1client.CoreV1Interface
	OAuthClient  oauthclient.Interface
	ConfigClient configclient.Interface
	ApiClient    kclient.Interface
}

// NewTools construct a set of ClientTools for the given config
func NewTools(cfg *rest.Config) (*ClientTools, error) {
	apiClient, err := kclient.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	coreClient := apiClient.CoreV1()

	configClient, err := configclient.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	oauthClient, err := oauthclient.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &ClientTools{
		CoreClient:   coreClient,
		OAuthClient:  oauthClient,
		ConfigClient: configClient,
		ApiClient:    apiClient,
	}, nil
}
