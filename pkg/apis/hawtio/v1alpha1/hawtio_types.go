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
	Version string     `json:"version,omitempty"`
	RBAC    HawtioRBAC `json:"rbac,omitempty"`
	// The compute resources
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// The RBAC configuration
type HawtioRBAC struct {
	// Whether RBAC is enabled. Defaults to true.
	Enabled *bool `json:"enabled,omitempty"`
	// The name of the ConfigMap that contains the ACL definition.
	ConfigMap string `json:"configMap,omitempty"`
}

// Reports the observed state of Hawtio
type HawtioStatus struct {
	// The Hawtio console container image
	Image string      `json:"image,omitempty"`
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

// HawtioConfig defines the hawtconfig.json file structure
type HawtioConfig struct {
	About          HawtioAbout    `json:"about"`
	Branding       HawtioBranding `json:"branding"`
	Online         HawtioOnline   `json:"online"`
	DisabledRoutes []string       `json:"disabledRoutes"`
}

type HawtioBranding struct {
	AppName    string `json:"appName"`
	AppLogoURL string `json:"appLogoUrl"`
	CSS        string `json:"css"`
	Favicon    string `json:"favicon"`
}

type HawtioAbout struct {
	Title          string              `json:"title"`
	ProductInfos   []HawtioProductInfo `json:"productInfo"`
	AdditionalInfo string              `json:"additionalInfo"`
	Copyright      string              `json:"copyright"`
	ImgSrc         string              `json:"imgSrc"`
}

type HawtioProductInfo struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type HawtioOnline struct {
	ProjectSelector string `json:"projectSelector,omitempty"`
	ConsoleLink     struct {
		Text              string `json:"text"`
		Section           string `json:"section"`
		ImageRelativePath string `json:"imageRelativePath"`
	} `json:"consoleLink"`
}

func init() {
	SchemeBuilder.Register(&Hawtio{}, &HawtioList{})
}
