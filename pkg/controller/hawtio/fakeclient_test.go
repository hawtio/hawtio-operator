package hawtio

import (
	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"testing"
)

//buildReconcileWithFakeClientWithMocks return reconcile with fake client, schemes and mock objects
func buildReconcileWithFakeClientWithMocks(objs []runtime.Object, t *testing.T) *ReconcileHawtio {

	registerObjs := []runtime.Object{&corev1.Service{}, &corev1.ServiceList{}, &appsv1.Deployment{}, &appsv1.DeploymentList{}, &corev1.ConfigMapList{}, &corev1.Pod{}, &routev1.Route{}, &routev1.RouteList{}}
	registerObjs = append(registerObjs)
	hawtiov1alpha1.SchemeBuilder.Register(registerObjs...)
	hawtiov1alpha1.SchemeBuilder.Register()

	scheme, err := hawtiov1alpha1.SchemeBuilder.Build()
	if err != nil {
		assert.Fail(t, "unable to build scheme")
	}

	client := fake.NewFakeClientWithScheme(scheme, objs...)

	return &ReconcileHawtio{
		scheme: scheme,
		client: client,
	}

}
