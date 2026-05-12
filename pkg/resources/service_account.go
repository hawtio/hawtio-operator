package resources

import (
	"encoding/json"

	"github.com/go-logr/logr"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	oauthv1 "github.com/openshift/api/oauth/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/hawtio/hawtio-operator/pkg/util"
)

func NewDefaultServiceAccount(hawtio *hawtiov2.Hawtio) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hawtio.Name,
			Namespace: hawtio.Namespace,
		},
	}
}

func NewServiceAccount(hawtio *hawtiov2.Hawtio, log logr.Logger) (*corev1.ServiceAccount, error) {
	log.V(util.DebugLogLevel).Info("New service account")

	annotations := make(map[string]string)

	if hawtio.Spec.Type == hawtiov2.NamespaceHawtioDeploymentType {
		//
		// hawtio is in namespace mode so utilize sa as oauthclient
		//
		externalRoutes := hawtio.Spec.ExternalRoutes
		routes := append(externalRoutes, hawtio.Name)
		for _, name := range routes {
			ref, err := createRedirectReferenceString(name)
			if err != nil {
				return nil, err
			}
			annotations["serviceaccounts.openshift.io/oauth-redirecturi."+name] = "https://"
			annotations["serviceaccounts.openshift.io/oauth-redirectreference."+name] = ref
		}
	}

	labels := LabelsForHawtio(hawtio.Name)
	PropagateLabels(hawtio, labels, log)

	sa := NewDefaultServiceAccount(hawtio)
	sa.SetLabels(labels)
	sa.SetAnnotations(annotations)
	return sa, nil
}

func createRedirectReferenceString(name string) (string, error) {
	OAuthRedirectReference := &oauthv1.OAuthRedirectReference{
		Reference: oauthv1.RedirectReference{
			Kind: "Route",
			Name: name,
		},
	}
	ref, err := json.Marshal(OAuthRedirectReference)
	if err != nil {
		return "", err
	}
	return string(ref), err
}
