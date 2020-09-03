package hawtio

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	"github.com/hawtio/hawtio-operator/pkg/resources"
)

func TestNonWatchedResourceNameNotFound(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	objs := []runtime.Object{
		&hawtio,
	}

	r := buildReconcileWithFakeClientWithMocks(objs, t)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "doesn't exist",
			Namespace: hawtio.Namespace,
		},
	}

	result, err := r.Reconcile(request)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

func TestNonWatchedResourceNamespaceNotFound(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	objs := []runtime.Object{
		&hawtio,
	}

	r := buildReconcileWithFakeClientWithMocks(objs, t)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      hawtio.Name,
			Namespace: "doesn't exist",
		},
	}

	result, err := r.Reconcile(request)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

func TestHawtioController_Reconcile(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	objs := []runtime.Object{
		&hawtio,
	}

	r := buildReconcileWithFakeClientWithMocks(objs, t)

	NamespacedName := types.NamespacedName{Name: hawtio.Name, Namespace: hawtio.Namespace}
	request := reconcile.Request{NamespacedName: NamespacedName}

	// Created phase
	res, err := r.Reconcile(request)
	assert.NoError(t, err, "reconcile Error")
	assert.Equal(t, reconcile.Result{Requeue: true}, res)
	// Initialized phase
	res, err = r.Reconcile(request)
	assert.NoError(t, err, "reconcile Error")
	assert.Equal(t, reconcile.Result{Requeue: true}, res)
	// Deployed phase
	res, err = r.Reconcile(request)
	assert.NoError(t, err, "reconcile Error")
	assert.Equal(t, reconcile.Result{}, res)

	t.Run("hawtio-online", func(t *testing.T) {
		t.Run("check if the Hawtio has been created", func(t *testing.T) {
			hawtio := hawtiov1alpha1.Hawtio{}
			err = r.client.Get(context.TODO(), NamespacedName, &hawtio)
			require.NoError(t, err)
		})
		t.Run("check if the Deployment has been created", func(t *testing.T) {
			deployment := appsv1.Deployment{}
			err = r.client.Get(context.TODO(), NamespacedName, &deployment)
			require.NoError(t, err)
		})
		t.Run("check if the Container resources have been set", func(t *testing.T) {
			deployment := appsv1.Deployment{}
			err = r.client.Get(context.TODO(), NamespacedName, &deployment)
			require.NoError(t, err)

			container := deployment.Spec.Template.Spec.Containers[0]
			assert.Equal(t, container.Resources, hawtio.Spec.Resources)
		})
		t.Run("check if the environment variables have been set", func(t *testing.T) {
			deployment := appsv1.Deployment{}
			err = r.client.Get(context.TODO(), NamespacedName, &deployment)
			require.NoError(t, err)

			container := deployment.Spec.Template.Spec.Containers[0]
			assert.ElementsMatch(t, container.Env, []corev1.EnvVar{
				{
					Name:  resources.HawtioTypeEnvVar,
					Value: strings.ToLower(hawtiov1alpha1.NamespaceHawtioDeploymentType),
				},
				{
					Name: resources.HawtioNamespaceEnvVar,
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							APIVersion: "v1",
							FieldPath:  "metadata.namespace",
						},
					},
				},
				{
					Name:  resources.HawtioOAuthClientEnvVar,
					Value: hawtio.Name,
				},
				{
					Name: resources.HawtioRbacEnvVar,
				},
			})
		})
	})
}
