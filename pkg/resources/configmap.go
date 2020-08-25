package resources

import (
	"bytes"
	"encoding/json"
	"errors"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

const (
	hawtioConfigPath = "config/config.yaml"
	hawtioConfigKey  = "hawtconfig.json"
)

// GetHawtioConfig reads the console configuration from the config map
func GetHawtioConfig(configMap *corev1.ConfigMap) (*hawtiov1alpha1.Hawtconfig, error) {
	var config *hawtiov1alpha1.Hawtconfig

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

// Create NewConfigMapForCR method to create configmap
func NewConfigMapForCR(cr *hawtiov1alpha1.Hawtio) (*corev1.ConfigMap, error) {
	config, err := configForHawtio(cr)
	if err != nil {
		return nil, err
	}

	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
		Data: map[string]string{
			hawtioConfigKey: config,
		},
	}

	return configMap, nil
}

func configForHawtio(m *hawtiov1alpha1.Hawtio) (string, error) {
	data, err := util.LoadConfigFromFile(hawtioConfigPath)
	if err != nil {
		return "", err
	}

	var buff bytes.Buffer
	config := template.Must(template.New("config").Parse(string(data)))
	err = config.Execute(&buff, m.Spec)
	if err != nil {
		return "", err
	}

	return buff.String(), nil
}
