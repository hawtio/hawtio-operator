package resources

import (
	"encoding/json"

	"github.com/go-logr/logr"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	oauthv1 "github.com/openshift/api/oauth/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
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

func NewDefaultServiceAccountRole(hawtio *hawtiov2.Hawtio) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hawtio.Name,
			Namespace: hawtio.Namespace,
		},
	}
}

func NewServiceAccountRole(hawtio *hawtiov2.Hawtio, log logr.Logger) *rbacv1.Role {
	labels := LabelsForHawtio(hawtio.Name)
	PropagateLabels(hawtio, labels, log)

	role := NewDefaultServiceAccountRole(hawtio)
	role.SetLabels(labels)
	role.Rules = []rbacv1.PolicyRule{
		// Permission for the Certificate Expiry Check (CronJob)
		{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"get", "delete"},
		},
		// Any other future permissions to go here >>
	}

	return role
}

func NewDefaultServiceAccountRoleBinding(hawtio *hawtiov2.Hawtio) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hawtio.Name, // Simply "hawtio-online"
			Namespace: hawtio.Namespace,
		},
	}
}

func NewServiceAccountRoleBinding(hawtio *hawtiov2.Hawtio, log logr.Logger) *rbacv1.RoleBinding {
	labels := LabelsForHawtio(hawtio.Name)
	PropagateLabels(hawtio, labels, log)

	roleBinding := NewDefaultServiceAccountRoleBinding(hawtio)
	roleBinding.SetLabels(labels)
	roleBinding.Subjects = []rbacv1.Subject{
		{
			Kind:      "ServiceAccount",
			Name:      hawtio.Name, // "hawtio-online"
			Namespace: hawtio.Namespace,
		},
	}
	roleBinding.RoleRef = rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     hawtio.Name,
	}

	return roleBinding
}
