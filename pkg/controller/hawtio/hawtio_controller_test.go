package hawtio

import (
	"context"
	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"testing"
)

func TestNonWatchedResourceNameNotFound(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	objs := []runtime.Object{
		&HawtioInstance,
	}

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "doesn't exist",
			Namespace: HawtioInstance.Namespace,
		},
	}
	r := buildReconcileWithFakeClientWithMocks(objs, t)
	result, err := r.Reconcile(request)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

}

func TestNonWatchedResourceNamespaceNotFound(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	objs := []runtime.Object{
		&HawtioInstance,
	}

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      HawtioInstance.Name,
			Namespace: "doesn't exist",
		},
	}
	r := buildReconcileWithFakeClientWithMocks(objs, t)
	result, err := r.Reconcile(request)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

}

func TestHawtioController_Reconcile(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	objs := []runtime.Object{
		&HawtioInstance,
	}
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      HawtioInstance.Name,
			Namespace: HawtioInstance.Namespace,
		},
	}
	r := buildReconcileWithFakeClientWithMocks(objs, t)

	res, err := r.Reconcile(request)
	assert.NoError(t, err, "reconcile Error ")
	assert.Equal(t, reconcile.Result{}, res)
	NamespacedName := types.NamespacedName{Name: HawtioInstance.Name, Namespace: HawtioInstance.Namespace}
	t.Run("hawtio-online", func(t *testing.T) {
		t.Run("cluster", func(t *testing.T) {
			hawtio := hawtiov1alpha1.Hawtio{}
			err = r.client.Get(context.TODO(), NamespacedName, &hawtio)
			require.NoError(t, err)
		})
		deployment := &appsv1.Deployment{}
		t.Run("check if the Deployment has been created", func(t *testing.T) {
			deploymentName := types.NamespacedName{Name: HawtioInstance.Name, Namespace: HawtioInstance.Namespace}
			err = r.client.Get(context.TODO(), deploymentName, deployment)

			require.NoError(t, err)

		})

	})

}
