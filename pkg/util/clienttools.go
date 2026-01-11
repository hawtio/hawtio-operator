package util

import (
	"fmt"
	"os"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type ClientTools struct {
	restConfig    *rest.Config
	runtimeClient *client.Client
	manager       manager.Manager
	dynamicClient dynamic.Interface
	apiClient     kubernetes.Interface
}

func ExitOnError(err error) {
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}

func (ck *ClientTools) RestConfig() *rest.Config {
	if ck.restConfig == nil {
		config, err := config.GetConfig()
		ExitOnError(err)
		ck.restConfig = config
	}

	return ck.restConfig
}

func (ck *ClientTools) Manager() manager.Manager {

	var namespaces map[string]cache.Config

	mgr, err := manager.New(ck.restConfig, manager.Options{
		Cache: cache.Options{
			DefaultNamespaces: namespaces,
		},
	})
	if err != nil {
		ExitOnError(err)
	}
	ck.manager = mgr
	return ck.manager
}

func (ck *ClientTools) RuntimeClient() (c client.Client, err error) {
	if ck.runtimeClient == nil {

		_, err := client.New(ck.restConfig, client.Options{})
		if err != nil {
			log.Error(err, "Unable to create the client")
			return nil, err
		}

		s := ck.manager.GetScheme()

		// Register
		options := client.Options{
			Scheme: s,
		}

		cl, err := client.New(ck.RestConfig(), options)
		if err != nil {
			return nil, err
		}
		ck.runtimeClient = &cl
	}

	return *ck.runtimeClient, nil
}

func (ck *ClientTools) SetRuntimeClient(c client.Client) {
	ck.runtimeClient = &c
}

func (ck *ClientTools) DynamicClient() (c dynamic.Interface, err error) {
	if ck.dynamicClient == nil {
		dyncl, err := dynamic.NewForConfig(ck.RestConfig())
		if err != nil {
			return nil, err
		}
		ck.dynamicClient = dyncl
	}
	return ck.dynamicClient, nil
}

func (ck *ClientTools) SetDynamicClient(d dynamic.Interface) {
	ck.dynamicClient = d
}

func (ck *ClientTools) ApiClient() (kubernetes.Interface, error) {
	if ck.apiClient == nil {
		apicl, err := kubernetes.NewForConfig(ck.RestConfig())
		if err != nil {
			return nil, err
		}
		ck.apiClient = apicl
	}
	return ck.apiClient, nil
}

func (ck *ClientTools) SetApiClient(a kubernetes.Interface) {
	ck.apiClient = a
}
