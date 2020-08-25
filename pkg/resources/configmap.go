package resources

import (
	"bytes"
	"fmt"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	osutil "github.com/hawtio/hawtio-operator/pkg/openshift/util"
)

const (
	hawtioConfigPath = "config/config.yaml"
)

// Create NewConfigMapForCR method to create configmap
func NewConfigMapForCR(cr *hawtiov1alpha1.Hawtio) *corev1.ConfigMap {
	config := configForHawtio(cr)
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
			"hawtconfig.json": config,
		},
	}

	return configMap
}

func configForHawtio(m *hawtiov1alpha1.Hawtio) string {
	data, err := osutil.LoadConfigFromFile(hawtioConfigPath)
	if err != nil {
		fmt.Errorf("error reading config file: %s")
	}

	var buff bytes.Buffer
	config := template.Must(template.New("config").Parse(string(data)))
	config.Execute(&buff, m.Spec)
	return buff.String()
}
