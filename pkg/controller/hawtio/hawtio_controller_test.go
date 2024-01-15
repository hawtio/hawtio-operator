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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hawtiov1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1"
	"github.com/hawtio/hawtio-operator/pkg/resources"
)

func TestNonWatchedResourceNameNotFound(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

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

	result, err := r.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

func TestNonWatchedResourceNamespaceNotFound(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

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

	result, err := r.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

func TestHawtioController_Reconcile(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	objs := []runtime.Object{
		&hawtio,
	}

	r := buildReconcileWithFakeClientWithMocks(objs, t)

	NamespacedName := types.NamespacedName{Name: hawtio.Name, Namespace: hawtio.Namespace}
	request := reconcile.Request{NamespacedName: NamespacedName}

	// Created phase
	res, err := r.Reconcile(context.TODO(), request)
	assert.NoError(t, err, "reconcile Error")
	assert.Equal(t, reconcile.Result{Requeue: true}, res)
	// Initialized phase
	res, err = r.Reconcile(context.TODO(), request)
	assert.NoError(t, err, "reconcile Error")
	assert.Equal(t, reconcile.Result{Requeue: true}, res)
	// Deployed phase
	res, err = r.Reconcile(context.TODO(), request)
	assert.NoError(t, err, "reconcile Error")
	assert.Equal(t, reconcile.Result{}, res)

	t.Run("hawtio-online", func(t *testing.T) {
		t.Run("check if the Hawtio has been created", func(t *testing.T) {
			hawtio := hawtiov1.Hawtio{}
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
		t.Run("check if the ConfigMap has been created", func(t *testing.T) {
			configMap := corev1.ConfigMap{}
			err = r.client.Get(context.TODO(), NamespacedName, &configMap)
			require.NoError(t, err)

			config, err := resources.GetHawtioConfig(&configMap)
			require.NoError(t, err)

			assert.Equal(t, config, &hawtiov1.HawtioConfig{
				About: hawtiov1.HawtioAbout{
					AdditionalInfo: "The Hawtio console eases the discovery and management of 'hawtio-enabled' applications deployed on OpenShift.",
					Title:          "Hawtio Console",
				},
				Branding: hawtiov1.HawtioBranding{
					AppLogoURL: "img/hawtio-logo.svg",
					AppName:    "Hawtio Console",
				},
				Online: hawtiov1.HawtioOnline{
					ConsoleLink: hawtiov1.HawtioConsoleLink{
						ImageRelativePath: "/online/img/favicon.ico",
						Section:           "Hawtio",
						Text:              "Hawtio Console",
					},
					ProjectSelector: "!openshift.io/run-level,!openshift.io/cluster-monitoring",
				},
			})
		})
		t.Run("check if the environment variables have been set", func(t *testing.T) {
			deployment := appsv1.Deployment{}
			err = r.client.Get(context.TODO(), NamespacedName, &deployment)
			require.NoError(t, err)

			container := deployment.Spec.Template.Spec.Containers[0]
			assert.ElementsMatch(t, container.Env, []corev1.EnvVar{
				{
					Name:  resources.HawtioTypeEnvVar,
					Value: strings.ToLower(string(hawtiov1.NamespaceHawtioDeploymentType)),
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
					Name:  resources.HawtioRbacEnvVar,
					Value: "",
				},
				{
					Name:  resources.HawtioAuthEnvVar,
					Value: "form",
				},
			})
		})
	})
}
