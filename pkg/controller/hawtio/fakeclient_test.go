package hawtio

import (
	"testing"

	"github.com/stretchr/testify/assert"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	routev1 "github.com/openshift/api/route/v1"

	fakeconfig "github.com/openshift/client-go/config/clientset/versioned/fake"
	fakeoauth "github.com/openshift/client-go/oauth/clientset/versioned/fake"

	"github.com/hawtio/hawtio-operator/pkg/apis"
)

//buildReconcileWithFakeClientWithMocks return *ReconcileHawtio with fake client, scheme and mock objects
func buildReconcileWithFakeClientWithMocks(objs []runtime.Object, t *testing.T) *ReconcileHawtio {
	var scheme = runtime.NewScheme()

	err := corev1.AddToScheme(scheme)
	if err != nil {
		assert.Fail(t, "unable to build scheme")
	}

	err = appsv1.AddToScheme(scheme)
	if err != nil {
		assert.Fail(t, "unable to build scheme")
	}

	err = routev1.Install(scheme)
	if err != nil {
		assert.Fail(t, "unable to build scheme")
	}

	err = apis.AddToScheme(scheme)
	if err != nil {
		assert.Fail(t, "unable to build scheme")
	}

	client := fake.NewFakeClientWithScheme(scheme, objs...)

	return &ReconcileHawtio{
		scheme:       scheme,
		client:       client,
		configClient: fakeconfig.NewSimpleClientset(),
		oauthClient:  fakeoauth.NewSimpleClientset(),
	}
}
