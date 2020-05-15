package configmaps

import (
	"bytes"
	"fmt"

	osutil "github.com/hawtio/hawtio-operator/pkg/openshift/util"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"text/template"
)

// Create NewConfigMapForCR method to create configmap
const (
	hawtioConfigPath = "config/config.yaml"
)

func NewConfigMapForCR(m *hawtiov1alpha1.Hawtio) *corev1.ConfigMap {
	config := ConfigForHawtio(m)
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Data: map[string]string{
			"hawtconfig.json": config,
		},
	}

	return configMap
}

func ConfigForHawtio(m *hawtiov1alpha1.Hawtio) string {

	data, err := osutil.LoadConfigFromFile(hawtioConfigPath)
	if err != nil {
		fmt.Errorf("error reading config file: %s")
	}

	var buff bytes.Buffer
	hawtioconfig := template.Must(template.New("hawtioconfig").Parse(string(data)))
	hawtioconfig.Execute(&buff, m.Spec)
	return buff.String()
}
