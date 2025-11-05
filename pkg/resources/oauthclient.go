package resources

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	oauthv1 "github.com/openshift/api/oauth/v1"
)

const OAuthClientName = "hawtio"

func NewDefaultOAuthClient(name string) *oauthv1.OAuthClient {
	return &oauthv1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func NewOAuthClient(name string) *oauthv1.OAuthClient {
	oAuthClient := NewDefaultOAuthClient(name)
	oAuthClient.GrantMethod = oauthv1.GrantHandlerAuto

	return oAuthClient
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
