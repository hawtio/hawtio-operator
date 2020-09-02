package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// HawtioDeploymentType defines the possible deployment types
type HawtioDeploymentType = string

const (
	// ClusterHawtioDeploymentType is the deployment type capable of discovering
	// and managing applications across all namespaces the authenticated user
	// has access to.
	ClusterHawtioDeploymentType HawtioDeploymentType = "Cluster"

	// NamespaceHawtioDeploymentType is the deployment type capable of discovering
	// and managing applications within the deployment namespace.
	NamespaceHawtioDeploymentType HawtioDeploymentType = "Namespace"
)

// HawtioSpec defines the desired state of Hawtio
type HawtioSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	Type          HawtioDeploymentType        `json:"type,omitempty"`
	Replicas      *int32                      `json:"replicas,omitempty"`
	RouteHostName string                      `json:"routeHostName,omitempty"`
	Version       string                      `json:"version,omitempty"`
	RBAC          HawtioRBAC                  `json:"rbac,omitempty"`
	Resources     corev1.ResourceRequirements `json:"resources,omitempty"`
}

// The RBAC configuration
type HawtioRBAC struct {
	// Whether RBAC is enabled.
	// Default is true.
	Enabled *bool `json:"enabled,omitempty"`
	// The name of the ConfigMap that contains the ACL definition.
	ConfigMap string `json:"configMap,omitempty"`
}

// HawtioStatus defines the observed state of Hawtio
type HawtioStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	Image    string      `json:"image,omitempty"`
	Phase    HawtioPhase `json:"phase,omitempty"`
	URL      string      `json:"URL,omitempty"`
	Replicas int32       `json:"replicas,omitempty"`
	Selector string      `json:"selector,omitempty"`
}

// HawtioPhase --
type HawtioPhase string

const (
	// HawtioPhaseInitialized --
	HawtioPhaseInitialized HawtioPhase = "Initialized"
	// HawtioPhaseDeployed --
	HawtioPhaseDeployed HawtioPhase = "Deployed"
	// HawtioPhaseFailed --
	HawtioPhaseFailed HawtioPhase = "Failed"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Hawtio is the Schema for the hawtios API
// +k8s:openapi-gen=true
type Hawtio struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HawtioSpec   `json:"spec,omitempty"`
	Status HawtioStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

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
	DisabledRoutes []string       `json:"disabledRoutes"`
}

type HawtioBranding struct {
	AppName     string `json:"appName"`
	AppLogoURL  string `json:"appLogoUrl"`
	ConsoleLink struct {
		Text              string `json:"text"`
		Section           string `json:"section"`
		ImageRelativePath string `json:"imageRelativePath"`
	} `json:"consoleLink"`
	CSS     string `json:"css"`
	Favicon string `json:"favicon"`
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

func init() {
	SchemeBuilder.Register(&Hawtio{}, &HawtioList{})
}
