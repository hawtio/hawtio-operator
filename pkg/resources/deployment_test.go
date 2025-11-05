package resources

import (
	"testing"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	"github.com/hawtio/hawtio-operator/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestGetServingCertificateMountPath(t *testing.T) {
	// version 'latest' should pass
	path, err := getServingCertificateMountPath("latest", "< 1.2.0")
	assert.NoError(t, err)
	assert.Equal(t, serviceSigningSecretVolumeMountPath, path)

	// a standard version should pass
	path, err = getServingCertificateMountPath("1.0.0", "< 1.2.0")
	assert.NoError(t, err)
	assert.Equal(t, serviceSigningSecretVolumeMountPathLegacy, path)

	// any arbitrary tag name as a version should also pass
	path, err = getServingCertificateMountPath("test", "< 1.2.0")
	assert.NoError(t, err)
	assert.Equal(t, serviceSigningSecretVolumeMountPath, path)
}

type Expected struct {
	onlineLogLevel  string
	gatewayLogLevel string
	maskIP          string
}

func findEnvVar(envs []corev1.EnvVar, name string) (string, bool) {
	for _, env := range envs {
		if env.Name == name {
			return env.Value, true
		}
	}
	return "", false
}

func TestNewDeploymentLogging(t *testing.T) {
	apiSpec := &capabilities.ApiServerSpec{
		IsOpenShift4: true,
	}
	openShiftConsoleURL := ""
	configMapVersion := ""
	clientCertSecretVersion := ""
	buildVariables := util.BuildVariables{
		ImageRepository:        "quay.io/hawtio/online",
		GatewayImageRepository: "quay.io/hawtio/online-gateway",
		ImageVersion:           "2.3.0",
		GatewayImageVersion:    "2.3.0",
	}
	log := logr.Discard()

	testCases := []struct {
		name     string
		hawtio   *hawtiov2.Hawtio
		expected Expected
	}{
		{
			"Default Hawtio",
			&hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hawtio-online",
					Namespace: "hawtio",
				},
				Spec: hawtiov2.HawtioSpec{},
			},
			Expected{
				onlineLogLevel:  "info",
				gatewayLogLevel: "info",
			},
		},
		{
			"Hawtio Log Level Debug",
			&hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hawtio-online",
					Namespace: "hawtio",
				},
				Spec: hawtiov2.HawtioSpec{
					Logging: hawtiov2.HawtioLogging{
						OnlineLogLevel:  "debug",
						GatewayLogLevel: "debug",
					},
				},
			},
			Expected{
				onlineLogLevel:  "debug",
				gatewayLogLevel: "debug",
			},
		},
		{
			"Hawtio Log Level Diff Values",
			&hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hawtio-online",
					Namespace: "hawtio",
				},
				Spec: hawtiov2.HawtioSpec{
					Logging: hawtiov2.HawtioLogging{
						OnlineLogLevel:  "crit",
						GatewayLogLevel: "trace",
					},
				},
			},
			Expected{
				onlineLogLevel:  "crit",
				gatewayLogLevel: "trace",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			deployment, err := NewDeployment(tc.hawtio, apiSpec, openShiftConsoleURL, configMapVersion, clientCertSecretVersion, buildVariables, log)
			assert.NoError(t, err)

			onlineEnv := deployment.Spec.Template.Spec.Containers[0].Env
			onlineLogLvlValue, found := findEnvVar(onlineEnv, HawtioOnlineLogLvlEnvVar)
			assert.True(t, found)
			assert.Equal(t, tc.expected.onlineLogLevel, onlineLogLvlValue)

			gatewayEnv := deployment.Spec.Template.Spec.Containers[1].Env
			gatewayLogLvlValue, found := findEnvVar(gatewayEnv, GatewayLogLvlEnvVar)
			assert.True(t, found)
			assert.Equal(t, tc.expected.gatewayLogLevel, gatewayLogLvlValue)
		})
	}
}

func TestNewDeploymentMaskIP(t *testing.T) {
	apiSpec := &capabilities.ApiServerSpec{
		IsOpenShift4: true,
	}
	openShiftConsoleURL := ""
	configMapVersion := ""
	clientCertSecretVersion := ""
	buildVariables := util.BuildVariables{
		ImageRepository:        "quay.io/hawtio/online",
		GatewayImageRepository: "quay.io/hawtio/online-gateway",
		ImageVersion:           "2.3.0",
		GatewayImageVersion:    "2.3.0",
	}
	log := logr.Discard()

	testCases := []struct {
		name     string
		hawtio   *hawtiov2.Hawtio
		expected Expected
	}{
		{
			"Default Hawtio",
			&hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hawtio-online",
					Namespace: "hawtio",
				},
				Spec: hawtiov2.HawtioSpec{},
			},
			Expected{
				maskIP: "true",
			},
		},
		{
			"Hawtio Mask IP True",
			&hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hawtio-online",
					Namespace: "hawtio",
				},
				Spec: hawtiov2.HawtioSpec{
					Logging: hawtiov2.HawtioLogging{
						MaskIPAddresses: "true",
					},
				},
			},
			Expected{
				maskIP: "true",
			},
		},
		{
			"Hawtio Mask IP False",
			&hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hawtio-online",
					Namespace: "hawtio",
				},
				Spec: hawtiov2.HawtioSpec{
					Logging: hawtiov2.HawtioLogging{
						MaskIPAddresses: "false",
					},
				},
			},
			Expected{
				maskIP: "false",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			deployment, err := NewDeployment(tc.hawtio, apiSpec, openShiftConsoleURL, configMapVersion, clientCertSecretVersion, buildVariables, log)
			assert.NoError(t, err)

			gatewayEnv := deployment.Spec.Template.Spec.Containers[1].Env
			gatewayIPMask, found := findEnvVar(gatewayEnv, GatewayMaskIPEnvVar)
			assert.True(t, found)
			assert.Equal(t, tc.expected.maskIP, gatewayIPMask)
		})
	}
}
