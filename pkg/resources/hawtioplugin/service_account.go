package hawtioplugin

import (
	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hawtiopluginv1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtioplugin/v1"
	resources "github.com/hawtio/hawtio-operator/pkg/resources/common"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

const (
	// ServiceAccountName is the name of the service account
	ServiceAccountName = "hawtio-console-plugin"
)

func NewDefaultServiceAccount(namespace string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceAccountName,
			Namespace: namespace,
		},
	}
}

func NewServiceAccount(namespace string, hawtioPlugin *hawtiopluginv1.HawtioPlugin, log logr.Logger) (*corev1.ServiceAccount, error) {
	log.V(util.DebugLogLevel).Info("New service account")

	labels := resources.LabelsForHawtio(hawtioPlugin.Name)
	resources.PropagateLabels(hawtioPlugin, labels, log)

	sa := NewDefaultServiceAccount(namespace)
	sa.SetLabels(labels)
	return sa, nil
}
