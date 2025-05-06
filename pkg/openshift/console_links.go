package openshift

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	consolev1 "github.com/openshift/api/console/v1"
	routev1 "github.com/openshift/api/route/v1"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
)

// defaultConsoleLinkText Default text for console launcher link
const defaultConsoleLinkText = "HawtIO Console"

// NewApplicationMenuLink creates an ApplicationMenu ConsoleLink instance
func NewApplicationMenuLink(name string, route *routev1.Route, config *hawtiov2.HawtioConfig) *consolev1.ConsoleLink {
	consoleLink := &consolev1.ConsoleLink{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"app": "hawtio"},
		},
		Spec: consolev1.ConsoleLinkSpec{
			Location:        consolev1.ApplicationMenu,
			ApplicationMenu: &consolev1.ApplicationMenuSpec{},
		},
	}

	UpdateApplicationMenuLink(consoleLink, route, config)

	return consoleLink
}

// UpdateApplicationMenuLink updates the ApplicationMenu ConsoleLink properties
func UpdateApplicationMenuLink(consoleLink *consolev1.ConsoleLink, route *routev1.Route, config *hawtiov2.HawtioConfig) {
	consoleLink.Spec.Location = consolev1.ApplicationMenu
	consoleLink.Spec.Link.Text = config.Online.ConsoleLink.Text

	if len(consoleLink.Spec.Link.Text) == 0 {
		consoleLink.Spec.Link.Text = defaultConsoleLinkText
	}

	consoleLink.Spec.Link.Href = "https://" + route.Spec.Host

	if consoleLink.Spec.ApplicationMenu == nil {
		consoleLink.Spec.ApplicationMenu = &consolev1.ApplicationMenuSpec{}
	}
	consoleLink.Spec.ApplicationMenu.Section = config.Online.ConsoleLink.Section
	consoleLink.Spec.ApplicationMenu.ImageURL = "https://" + route.Spec.Host + config.Online.ConsoleLink.ImageRelativePath
}

// NewNamespaceDashboardLink creates a NamespaceDashboard ConsoleLink instance
func NewNamespaceDashboardLink(name string, namespace string, route *routev1.Route, config *hawtiov2.HawtioConfig) *consolev1.ConsoleLink {
	consoleLink := &consolev1.ConsoleLink{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"app": "hawtio"},
		},
		Spec: consolev1.ConsoleLinkSpec{
			Location: consolev1.NamespaceDashboard,
			NamespaceDashboard: &consolev1.NamespaceDashboardSpec{
				Namespaces: []string{namespace},
			},
		},
	}

	UpdateNamespaceDashboardLink(consoleLink, route, config)

	return consoleLink
}

// UpdateNamespaceDashboardLink updates the NamespaceDashboard ConsoleLink properties
func UpdateNamespaceDashboardLink(consoleLink *consolev1.ConsoleLink, route *routev1.Route, config *hawtiov2.HawtioConfig) {
	consoleLink.Spec.Location = consolev1.NamespaceDashboard
	consoleLink.Spec.Link.Text = config.Online.ConsoleLink.Text

	if len(consoleLink.Spec.Link.Text) == 0 {
		consoleLink.Spec.Link.Text = defaultConsoleLinkText
	}

	consoleLink.Spec.Link.Href = "https://" + route.Spec.Host
	// ApplicationMenu can be set when the Hawtio type changes from 'cluster' to 'namespace'
	consoleLink.Spec.ApplicationMenu = nil
}
