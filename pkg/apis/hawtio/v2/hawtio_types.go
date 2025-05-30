package v2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make k8s-generate" to regenerate code after modifying this file

// HawtioDeploymentType defines the possible deployment types
// +kubebuilder:validation:Enum=Cluster;Namespace
type HawtioDeploymentType string

const (
	// ClusterHawtioDeploymentType is the deployment type capable of discovering
	// and managing applications across all namespaces the authenticated user
	// has access to.
	ClusterHawtioDeploymentType HawtioDeploymentType = "Cluster"

	// NamespaceHawtioDeploymentType is the deployment type capable of discovering
	// and managing applications within the deployment namespace.
	NamespaceHawtioDeploymentType HawtioDeploymentType = "Namespace"
)

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=hawtios,scope=Namespaced,shortName=hwt;hio;hawt,categories=hawtio
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Creation phase"
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.status.image`,description="Console image"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`,description="Console phase"
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.URL`,description="Console URL"

// Hawtio is the Schema for the Hawtio Console API.
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
	// The configuration for which metadata on Hawtio custom resources to propagate to
	// generated resources such as deployments, pods, services, and routes.
	MetadataPropagation HawtioMetadataPropagation `json:"metadataPropagation,omitempty"`
	// The edge host name of the route that exposes the Hawtio service
	// externally. If not specified, it is automatically generated and
	// is of the form:
	// <name>[-<namespace>].<suffix>
	// where <suffix> is the default routing sub-domain as configured for
	// the cluster.
	// Note that the operator will recreate the route if the field is emptied,
	// so that the host is re-generated.
	RouteHostName string `json:"routeHostName,omitempty"`
	// Custom certificate configuration for the route
	Route HawtioRoute `json:"route,omitempty"`
	// List of external route names that will be annotated by the operator to access the console using the routes
	ExternalRoutes []string `json:"externalRoutes,omitempty"`
	// The Hawtio console container image version.
	// Deprecated: Remains for legacy purposes in respect of older
	// operators (<1.0.0) still requiring it for their installs
	Version string `json:"version,omitempty"`
	// The authentication configuration
	Auth HawtioAuth `json:"auth,omitempty"`
	// The Nginx runtime configuration
	Nginx HawtioNginx `json:"nginx,omitempty"`
	// The RBAC configuration
	RBAC HawtioRBAC `json:"rbac,omitempty"`
	// The Hawtio console compute resources
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// The Hawtio console configuration
	Config HawtioConfig `json:"config,omitempty"`
}

// The configuration for which metadata on Hawtio custom resources to propagate to
// generated resources such as deployments, pods, services, and routes.
type HawtioMetadataPropagation struct {
	// Annotations to propagate
	Annotations []string `json:"annotations,omitempty"`
	// Labels to propagate
	Labels []string `json:"labels,omitempty"`
}

type HawtioRoute struct {
	// Name of the TLS secret with the custom certificate used for the route TLS termination
	CertSecret corev1.LocalObjectReference `json:"certSecret,omitempty"`
	// Ca certificate secret key selector
	CaCert corev1.SecretKeySelector `json:"caCert,omitempty"`
}

// HawtioAuth The authentication configuration
type HawtioAuth struct {
	// Use SSL for internal communication
	// +kubebuilder:default=true
	// +kubebuilder:validation:Required
	// +validation:Required
	// +required
	// +default=true
	InternalSSL *bool `json:"internalSSL,omitempty"`
	// The generated client certificate CN
	ClientCertCommonName string `json:"clientCertCommonName,omitempty"`
	// The generated client certificate expiration date
	ClientCertExpirationDate *metav1.Time `json:"clientCertExpirationDate,omitempty"`
	// CronJob schedule that defines how often the expiry of the certificate will be checked.
	// Client rotation isn't enabled if the schedule isn't set.
	ClientCertCheckSchedule string `json:"clientCertCheckSchedule,omitempty"`
	// The duration in hours before the expiration date, during which the certification can be rotated.
	// The default is set to 24 hours.
	ClientCertExpirationPeriod int `json:"clientCertExpirationPeriod,omitempty"`
}

// The Nginx runtime configuration
type HawtioNginx struct {
	// The buffer size for reading client request body. Defaults to `256k`.
	ClientBodyBufferSize string `json:"clientBodyBufferSize,omitempty"`
	// The number and size of the buffers used for reading a response from
	// the proxied server, for a single connection. Defaults to `16 128k`.
	ProxyBuffers string `json:"proxyBuffers,omitempty"`
	// The size of the buffer used for storing the response body of a subrequest.
	// Defaults to `10m`.
	SubrequestOutputBufferSize string `json:"subrequestOutputBufferSize,omitempty"`
}

// The RBAC configuration
type HawtioRBAC struct {
	// The name of the ConfigMap that contains the ACL definition.
	ConfigMap string `json:"configMap,omitempty"`
	// Disable performance improvement brought by RBACRegistry and revert to the classic behavior. Defaults to `false`.
	DisableRBACRegistry *bool `json:"disableRBACRegistry,omitempty"`
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

// HawtioConfig defines the hawtconfig.json structure.
// This type reflects only part of the whole definitions hawtconfig.json can have.
// Only the options that may be used by the operator are defined.
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

// NewHawtio initialise the defaults of a new Hawtio
func NewHawtio() *Hawtio {
	hawtio := &Hawtio{}

	// Set SSL to true by default
	b := true
	hawtio.Spec.Auth.InternalSSL = &b

	return hawtio
}

func init() {
	SchemeBuilder.Register(&Hawtio{}, &HawtioList{})
}
