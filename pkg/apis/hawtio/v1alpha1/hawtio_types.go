package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make k8s-generate" to regenerate code after modifying this file

// HawtioDeploymentType defines the possible deployment types
// +kubebuilder:validation:Enum=cluster;namespace
type HawtioDeploymentType string

const (
	// ClusterHawtioDeploymentType is the deployment type capable of discovering
	// and managing applications across all namespaces the authenticated user
	// has access to.
	ClusterHawtioDeploymentType HawtioDeploymentType = "cluster"

	// NamespaceHawtioDeploymentType is the deployment type capable of discovering
	// and managing applications within the deployment namespace.
	NamespaceHawtioDeploymentType HawtioDeploymentType = "namespace"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=hawtios,scope=Namespaced,categories=hawtio
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Creation phase"
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.status.image`,description="Console image"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`,description="Console phase"
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.URL`,description="Console URL"

// Hawtio Console
type Hawtio struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HawtioSpec   `json:"spec,omitempty"`
	Status HawtioStatus `json:"status,omitempty"`
}

// Defines the desired state of Hawtio
type HawtioSpec struct {
	// The deployment type. Defaults to cluster.
	// cluster: Hawtio is capable of discovering and managing
	// applications across all namespaces the authenticated user
	// has access to.
	// namespace: Hawtio is capable of discovering and managing
	// applications within the deployment namespace.
	Type HawtioDeploymentType `json:"type,omitempty"`
	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	Replicas *int32 `json:"replicas,omitempty"`
	// The edge host name of the route that exposes the Hawtio service
	// externally. If not specified, it is automatically generated and
	// is of the form:
	// <name>[-<namespace>].<suffix>
	// where <suffix> is the default routing sub-domain as configured for
	// the cluster.
	// Note that the operator will recreate the route if the field is emptied,
	// so that the host is re-generated.
	RouteHostName string `json:"routeHostName,omitempty"`
	// The Hawtio console container image version. Defaults to 'latest'.
	Version string `json:"version,omitempty"`
	// The authentication configuration
	Auth HawtioAuth `json:"auth,omitempty"`
	// The RBAC configuration
	RBAC HawtioRBAC `json:"rbac,omitempty"`
	// The Hawtio console compute resources
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// The Hawtio console configuration
	Config HawtioConfig `json:"config,omitempty"`
}

// The RBAC configuration
type HawtioRBAC struct {
	// Whether RBAC is enabled. Defaults to true.
	Enabled *bool `json:"enabled,omitempty"`
	// The name of the ConfigMap that contains the ACL definition.
	ConfigMap string `json:"configMap,omitempty"`
}

// The authentication configuration
type HawtioAuth struct {
	// The generated client certificate CN
	ClientCertCommonName string `json:"clientCertCommonName,omitempty"`
	// The generated client certificate expiration date
	ClientCertExpirationDate *metav1.Time `json:"clientCertExpirationDate,omitempty"`
}

// Reports the observed state of Hawtio
type HawtioStatus struct {
	// The Hawtio console container image
	Image string `json:"image,omitempty"`
	// The Hawtio deployment phase
	Phase HawtioPhase `json:"phase,omitempty"`
	// The Hawtio console route URL
	URL string `json:"URL,omitempty"`
	// The actual number of pods
	Replicas int32 `json:"replicas,omitempty"`
	// The label selector for the Hawtio pods
	Selector string `json:"selector,omitempty"`
}

// The Hawtio deployment phase
// +kubebuilder:validation:Enum=Initialized;Deployed;Failed
type HawtioPhase string

const (
	// HawtioPhaseInitialized --
	HawtioPhaseInitialized HawtioPhase = "Initialized"
	// HawtioPhaseDeployed --
	HawtioPhaseDeployed HawtioPhase = "Deployed"
	// HawtioPhaseFailed --
	HawtioPhaseFailed HawtioPhase = "Failed"
)

// +kubebuilder:object:root=true
// HawtioList contains a list of Hawtio
type HawtioList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Hawtio `json:"items"`
}

// HawtioConfig defines the hawtconfig.json structure
type HawtioConfig struct {
	// The information to be displayed in the About page
	About HawtioAbout `json:"about,omitempty"`
	// The UI branding
	Branding HawtioBranding `json:"branding,omitempty"`
	// The OpenShift related configuration
	Online HawtioOnline `json:"online,omitempty"`
	// Disables UI components with matching routes
	DisabledRoutes []string `json:"disabledRoutes,omitempty"`
}

// The UI branding
type HawtioBranding struct {
	// The application title, that usually displays in the Web browser tab.
	AppName string `json:"appName,omitempty"`
	// The URL of the logo, that displays in the navigation bar.
	// It can be a path, relative to the Hawtio status URL, or an absolute URL.
	AppLogoURL string `json:"appLogoUrl,omitempty"`
	// The URL of an external CSS stylesheet, that can be used to style the application.
	// It can be a path, relative to the Hawtio status URL, or an absolute URL.
	CSS string `json:"css,omitempty"`
	// The URL of the favicon, that usually displays in the Web browser tab.
	// It can be a path, relative to the Hawtio status URL, or an absolute URL.
	Favicon string `json:"favicon,omitempty"`
}

// The About page information
type HawtioAbout struct {
	// The title of the page
	Title string `json:"title,omitempty"`
	// List of product information
	ProductInfos []HawtioProductInfo `json:"productInfo,omitempty"`
	// The text for the description section
	AdditionalInfo string `json:"additionalInfo,omitempty"`
	// The text for the copyright section
	Copyright string `json:"copyright,omitempty"`
	// The image displayed in the page.
	// It can be a path, relative to the Hawtio status URL, or an absolute URL.
	ImgSrc string `json:"imgSrc,omitempty"`
}

// The product information displayed in the About page
type HawtioProductInfo struct {
	// The name of the product information
	Name string `json:"name"`
	// The value of the product information
	Value string `json:"value"`
}

// The OpenShift related configuration
type HawtioOnline struct {
	// The selector used to watch for projects.
	// It is only applicable when the Hawtio deployment type is equal to 'cluster'.
	// By default, all the projects the logged in user has access to are watched.
	// The string representation of the selector must be provided, as mandated by the `--selector`, or `-l`, options from the `kubectl get` command.
	// See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
	ProjectSelector string `json:"projectSelector,omitempty"`
	// The configuration for the OpenShift Web console link.
	// A link is added to the application menu when the Hawtio deployment is equal to 'cluster'.
	// Otherwise, a link is added to the Hawtio project dashboard.
	ConsoleLink HawtioConsoleLink `json:"consoleLink,omitempty"`
}

// The configuration for the OpenShift Web console link
type HawtioConsoleLink struct {
	// The text display for the link
	Text string `json:"text,omitempty"`
	// The section of the application menu in which the link should appear.
	//It is only applicable when the Hawtio deployment type is equal to 'cluster'.
	// +optional
	Section string `json:"section,omitempty"`
	// The path, relative to the Hawtio status URL, for the icon used in front of the link in the application menu.
	// It is only applicable when the Hawtio deployment type is equal to 'cluster'.
	// The image should be square and will be shown at 24x24 pixels.
	// +optional
	ImageRelativePath string `json:"imageRelativePath,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Hawtio{}, &HawtioList{})
}
