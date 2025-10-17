package resources

import (
	"encoding/json"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	oauthv1 "github.com/openshift/api/oauth/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewDefaultServiceAccountAsOauthClient(hawtio *hawtiov2.Hawtio) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hawtio.Name,
			Namespace: hawtio.Namespace,
		},
	}
}

func NewServiceAccountAsOauthClient(hawtio *hawtiov2.Hawtio) (*corev1.ServiceAccount, error) {
	externalRoutes := hawtio.Spec.ExternalRoutes
	annotations := make(map[string]string)
	routes := append(externalRoutes, hawtio.Name)
	for _, name := range routes {

		ref, err := createRedirectReferenceString(name)
		if err != nil {
			return nil, err
		}
		annotations["serviceaccounts.openshift.io/oauth-redirecturi."+name] = "https://"
		annotations["serviceaccounts.openshift.io/oauth-redirectreference."+name] = ref
	}

	labels := map[string]string{
		"app": "hawtio",
	}

	sa := NewDefaultServiceAccountAsOauthClient(hawtio)
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
