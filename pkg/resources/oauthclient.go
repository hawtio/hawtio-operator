package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	oauthv1 "github.com/openshift/api/oauth/v1"

	"github.com/hawtio/hawtio-operator/pkg/openshift/util"
)

func NewServiceAccountAsOauthClient(name string) (*corev1.ServiceAccount, error) {
	OAuthRedirectReference := &oauthv1.OAuthRedirectReference{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OAuthRedirectReference",
			APIVersion: "v1",
		},
		Reference: oauthv1.RedirectReference{
			Kind: "Route",
			Name: name,
		},
	}

	ref, err := util.Encode(OAuthRedirectReference)
	if err != nil {
		return nil, err
	}

	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
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

func NewOAuthClient(name string) *oauthv1.OAuthClient {
	oc := &oauthv1.OAuthClient{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OAuthClient",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		GrantMethod: oauthv1.GrantHandlerAuto,
	}
	return oc
}

func OauthClientContainsRedirectURI(oc *oauthv1.OAuthClient, uri string) (bool, int) {
	for i, u := range oc.RedirectURIs {
		if u == uri {
			return true, i
		}
	}
	return false, -1
}

func RemoveRedirectURIFromOauthClient(oc *oauthv1.OAuthClient, uri string) bool {
	ok, i := OauthClientContainsRedirectURI(oc, uri)
	if ok {
		oc.RedirectURIs = append(oc.RedirectURIs[:i], oc.RedirectURIs[i+1:]...)
		return true
	}
	return false
}
