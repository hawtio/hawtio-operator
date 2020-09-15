package resources

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	oauthv1 "github.com/openshift/api/oauth/v1"
)

func NewServiceAccountAsOauthClient(name string) (*corev1.ServiceAccount, error) {
	OAuthRedirectReference := &oauthv1.OAuthRedirectReference{
		Reference: oauthv1.RedirectReference{
			Kind: "Route",
			Name: name,
		},
	}

	ref, err := json.Marshal(OAuthRedirectReference)
	if err != nil {
		return nil, err
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app": "hawtio",
			},
			Annotations: map[string]string{
				"serviceaccounts.openshift.io/oauth-redirecturi.route":       "https://",
				"serviceaccounts.openshift.io/oauth-redirectreference.route": string(ref),
			},
		},
	}
	return sa, nil
}
