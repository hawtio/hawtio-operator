package hawtio

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/resources"
)

func TestNonWatchedResourceNameNotFound(t *testing.T) {
	hawtio := defaultHawtio
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	objs := []client.Object{
		hawtio, // Initialised in mocks_test
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
	hawtio := defaultHawtio
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	objs := []client.Object{
		hawtio, // Initialised in mocks_test
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

var plainHawtioEnvVars = []corev1.EnvVar{
	{
		Name:  resources.HawtioTypeEnvVar,
		Value: strings.ToLower(string(hawtiov2.NamespaceHawtioDeploymentType)),
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
		Name:  resources.HawtioAuthEnvVar,
		Value: "form",
	},
}

var sslHawtioEnvVars = append(plainHawtioEnvVars,
	corev1.EnvVar{
		Name:  resources.HawtioSSLKey,
		Value: resources.HawtioSSLKeyValue,
	},
	corev1.EnvVar{
		Name:  resources.HawtioSSLCert,
		Value: resources.HawtioSSLCertValue,
	},
)

var plainGatewayEnvVars = []corev1.EnvVar{
	{
		Name:  resources.GatewayWebSvrEnvVar,
		Value: "http://localhost:8080",
	},
	{
		Name:  resources.HawtioAuthEnvVar,
		Value: "form",
	},
}

var sslGatewayEnvVars = append(plainGatewayEnvVars[1:],
	corev1.EnvVar{
		Name:  resources.GatewayWebSvrEnvVar,
		Value: "https://localhost:8443",
	},
	corev1.EnvVar{
		Name:  resources.GatewaySSLKeyEnvVar,
		Value: "/etc/tls/private/serving/tls.key",
	},
	corev1.EnvVar{
		Name:  resources.GatewaySSLCertEnvVar,
		Value: "/etc/tls/private/serving/tls.crt",
	},
	corev1.EnvVar{
		Name:  resources.GatewaySSLCertCAEnvVar,
		Value: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
	},
)

type Expected struct {
	hawtioEnvVars  []corev1.EnvVar
	gatewayEnvVars []corev1.EnvVar
	connectScheme  resources.Connect
}

func TestHawtioController_Reconcile(t *testing.T) {

	var tests = []struct {
		name     string
		hawtio   *hawtiov2.Hawtio
		expected Expected
	}{
		{
			name:   "TestHawtioController_Reconcile_Default",
			hawtio: defaultHawtio,
			expected: Expected{
				hawtioEnvVars:  sslHawtioEnvVars,
				gatewayEnvVars: sslGatewayEnvVars,
				connectScheme:  resources.SSLConnect,
			},
		},
		{
			name:   "TestHawtioController_Reconcile_Plain",
			hawtio: plainHawtio,
			expected: Expected{
				hawtioEnvVars:  plainHawtioEnvVars,
				gatewayEnvVars: plainGatewayEnvVars,
				connectScheme:  resources.PlainConnect,
			},
		},
		{
			name:   "TestHawtioController_Reconcile_SSL",
			hawtio: sslHawtio,
			expected: Expected{
				hawtioEnvVars:  sslHawtioEnvVars,
				gatewayEnvVars: sslGatewayEnvVars,
				connectScheme:  resources.SSLConnect,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hawtio := tt.hawtio
			logf.SetLogger(zap.New(zap.UseDevMode(true)))
			objs := []client.Object{
				hawtio, // Initialised in mocks_test
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
					hawtio := hawtiov2.NewHawtio()
					err = r.client.Get(context.TODO(), NamespacedName, hawtio)

					require.NoError(t, err)
				})
				t.Run("check if the Deployment has been created", func(t *testing.T) {
					deployment := appsv1.Deployment{}
					err = r.client.Get(context.TODO(), NamespacedName, &deployment)
					require.NoError(t, err)

					assert.NotNil(t, deployment.Spec.Template.Spec)
					spec := deployment.Spec.Template.Spec

					assert.Len(t, spec.Containers, 2)
					for _, c := range spec.Containers {
						assert.Equal(t, c.ReadinessProbe.ProbeHandler.HTTPGet.Scheme,
							tt.expected.connectScheme.Protocol)

						assert.Equal(t, c.LivenessProbe.ProbeHandler.HTTPGet.Scheme,
							tt.expected.connectScheme.Protocol)

						assert.Len(t, c.Ports, 1)
						if c.Name == (hawtio.Name + "-gateway-container") {
							assert.Equal(t, c.Ports[0].ContainerPort, int32(3000))
						} else {
							assert.Equal(t, c.Ports[0].ContainerPort, tt.expected.connectScheme.Port)
						}
					}
				})
				t.Run("check if the Container resources have been set", func(t *testing.T) {
					deployment := appsv1.Deployment{}
					err = r.client.Get(context.TODO(), NamespacedName, &deployment)
					require.NoError(t, err)

					hawtioContainer := deployment.Spec.Template.Spec.Containers[0]
					assert.Equal(t, hawtioContainer.Resources, hawtio.Spec.Resources)

					gatewayContainer := deployment.Spec.Template.Spec.Containers[1]
					assert.Equal(t, gatewayContainer.Resources, hawtio.Spec.Resources)
				})
				t.Run("check if the ConfigMap has been created", func(t *testing.T) {
					configMap := corev1.ConfigMap{}
					err = r.client.Get(context.TODO(), NamespacedName, &configMap)
					require.NoError(t, err)

					config, err := resources.GetHawtioConfig(&configMap)
					require.NoError(t, err)

					assert.Equal(t, config, &hawtiov2.HawtioConfig{
						About: hawtiov2.HawtioAbout{
							AdditionalInfo: "The Hawtio console eases the discovery and management of 'hawtio-enabled' applications deployed on Kubernetes.",
							Title:          "Hawtio Console",
						},
						Branding: hawtiov2.HawtioBranding{
							AppLogoURL: "img/hawtio-logo.svg",
							AppName:    "Hawtio Console",
						},
						Online: hawtiov2.HawtioOnline{
							ConsoleLink: hawtiov2.HawtioConsoleLink{
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

					hawtioContainer := deployment.Spec.Template.Spec.Containers[0]
					assert.ElementsMatch(t, hawtioContainer.Env, tt.expected.hawtioEnvVars)

					gatewayContainer := deployment.Spec.Template.Spec.Containers[1]
					assert.ElementsMatch(t, gatewayContainer.Env, tt.expected.gatewayEnvVars)
				})
			})
		})
	}
}
