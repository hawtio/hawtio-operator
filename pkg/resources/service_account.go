package resources

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	oauthv1 "github.com/openshift/api/oauth/v1"
)

func NewServiceAccountAsOauthClient(name string, externalRoutes []string) (*corev1.ServiceAccount, error) {
	annotations := make(map[string]string)
	routes := append(externalRoutes, name)
	for _, name := range routes {

		ref, err := createRedirectReferenceString(name)
		if err != nil {
			return nil, err
		}
		annotations["serviceaccounts.openshift.io/oauth-redirecturi."+name] = "https://"
		annotations["serviceaccounts.openshift.io/oauth-redirectreference."+name] = ref
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app": "hawtio",
			},
			Annotations: annotations,
		},
	}
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
