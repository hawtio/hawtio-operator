package hawtio

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
)

func TestNonWatchedResourceNameNotFound(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	objs := []runtime.Object{
		&HawtioInstance,
	}

	r := buildReconcileWithFakeClientWithMocks(objs, t)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "doesn't exist",
			Namespace: HawtioInstance.Namespace,
		},
	}

	result, err := r.Reconcile(request)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

func TestNonWatchedResourceNamespaceNotFound(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	objs := []runtime.Object{
		&HawtioInstance,
	}

	r := buildReconcileWithFakeClientWithMocks(objs, t)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      HawtioInstance.Name,
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
		&HawtioInstance,
	}

	r := buildReconcileWithFakeClientWithMocks(objs, t)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      HawtioInstance.Name,
			Namespace: HawtioInstance.Namespace,
		},
	}

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

	NamespacedName := types.NamespacedName{Name: HawtioInstance.Name, Namespace: HawtioInstance.Namespace}
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
	})
}
