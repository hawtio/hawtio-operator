package resources

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

const (
	hawtioConfigPath = "config/config.yaml"
	hawtioConfigKey  = "hawtconfig.json"
)

// GetHawtioConfig reads the console configuration from the config map
func GetHawtioConfig(configMap *corev1.ConfigMap) (*hawtiov2.HawtioConfig, error) {
	var config *hawtiov2.HawtioConfig

	data, ok := configMap.Data[hawtioConfigKey]
	if !ok {
		return config, errors.New("did not find " + hawtioConfigKey + " in ConfigMap")
	}

	err := json.Unmarshal([]byte(data), &config)
	if err != nil {
		return config, err
	}

	return config, nil
}

func NewConfigMap(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec, log logr.Logger) (*corev1.ConfigMap, error) {
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Reconciling config map %s", hawtio.Name))

	config, err := configForHawtio(hawtio)
	if err != nil {
		return nil, err
	}

	if !apiSpec.IsOpenShift4 {
		config = strings.Replace(config, "OpenShift", "Kubernetes", -1)
	}

	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Hawtio config map: %s", config))

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hawtio.Name,
			Namespace: hawtio.Namespace,
		},
		Data: map[string]string{
			hawtioConfigKey: config,
		},
	}

	return configMap, nil
}

func configForHawtio(hawtio *hawtiov2.Hawtio) (string, error) {
	data, err := util.LoadConfigFromFile(hawtioConfigPath)
	if err != nil {
		return "", err
	}
	var defaultConfig interface{}
	err = json.Unmarshal(data, &defaultConfig)
	if err != nil {
		return "", err
	}

	data, err = json.Marshal(hawtio.Spec.Config)
	if err != nil {
		return "", err
	}
	var hawtioConfig interface{}
	err = json.Unmarshal(data, &hawtioConfig)
	if err != nil {
		return "", err
	}

	config := merge(hawtioConfig, defaultConfig)
	data, err = json.Marshal(config)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func merge(x1, x2 interface{}) interface{} {
	switch x1 := x1.(type) {
	case map[string]interface{}:
		x2, ok := x2.(map[string]interface{})
		if !ok {
			return x1
		}
		for k, v2 := range x2 {
			if v1, ok := x1[k]; ok {
				x1[k] = merge(v1, v2)
			} else {
				x1[k] = v2
			}
		}
	case nil:
		x2, ok := x2.(map[string]interface{})
		if ok {
			return x2
		}
	}
	return x1
}
