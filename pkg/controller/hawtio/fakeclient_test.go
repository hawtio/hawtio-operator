package hawtio

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	consolev1 "github.com/openshift/api/console/v1"
	oauthv1 "github.com/openshift/api/oauth/v1"
	routev1 "github.com/openshift/api/route/v1"
	fakeconfig "github.com/openshift/client-go/config/clientset/versioned/fake"
	fakeoauth "github.com/openshift/client-go/oauth/clientset/versioned/fake"
	networkingv1 "k8s.io/api/networking/v1"
	discoveryfake "k8s.io/client-go/discovery/fake"
	fakekube "k8s.io/client-go/kubernetes/fake"

	"github.com/hawtio/hawtio-operator/pkg/apis"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
)

//buildReconcileWithFakeClientWithMocks return *ReconcileHawtio with fake client, scheme and mock objects
func buildReconcileWithFakeClientWithMocks(objs []client.Object, t *testing.T) *ReconcileHawtio {
	var scheme = runtime.NewScheme()

	err := corev1.AddToScheme(scheme)
	if err != nil {
		assert.Fail(t, "unable to build scheme")
	}

	err = appsv1.AddToScheme(scheme)
	if err != nil {
		assert.Fail(t, "unable to build scheme")
	}

	err = rbacv1.AddToScheme(scheme)
	if err != nil {
		assert.Fail(t, "unable to build scheme")
	}

	err = routev1.Install(scheme)
	if err != nil {
		assert.Fail(t, "unable to build scheme")
	}

	err = networkingv1.AddToScheme(scheme)
	if err != nil {
		assert.Fail(t, "unable to build scheme")
	}

	err = apis.AddToScheme(scheme)
	if err != nil {
		assert.Fail(t, "unable to build scheme")
	}

	err = consolev1.AddToScheme(scheme)
	if err != nil {
		assert.Fail(t, "unable to build scheme")
	}

	err = oauthv1.AddToScheme(scheme)
	if err != nil {
		assert.Fail(t, "unable to build scheme")
	}

	client := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(objs...).
		WithObjects(objs...).
		Build()
	apiClient := fakekube.NewSimpleClientset()

	fd := apiClient.Discovery().(*discoveryfake.FakeDiscovery)
	fd.FakedServerVersion = &version.Info{
		Major: "4",
		Minor: "13",
	}

	configClient := fakeconfig.NewSimpleClientset()
	coreClient := apiClient.CoreV1()

	apiSpec, err := capabilities.APICapabilities(context.TODO(), apiClient, configClient)
	if err != nil {
		assert.Fail(t, "unable to define api capabilities")
	}

	return &ReconcileHawtio{
		scheme:       scheme,
		client:       client,
		configClient: configClient,
		coreClient:   coreClient,
		oauthClient:  fakeoauth.NewSimpleClientset(),
		apiClient:    apiClient,
		apiSpec:      apiSpec,
	}
}
