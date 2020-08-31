package resources

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	oauthv1 "github.com/openshift/api/oauth/v1"
)

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
