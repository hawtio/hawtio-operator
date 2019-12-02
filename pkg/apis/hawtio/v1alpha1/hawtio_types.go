package v1alpha1

import (
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
	Type          HawtioDeploymentType `json:"type,omitempty"`
	Replicas      int32                `json:"replicas,omitempty"`
	RouteHostName string               `json:"routeHostName,omitempty"`
	Version       string               `json:"version,omitempty"`
}

// HawtioStatus defines the observed state of Hawtio
type HawtioStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	Image string      `json:"image,omitempty"`
	Phase HawtioPhase `json:"phase,omitempty"`
	URL   string      `json:"URL,omitempty"`
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

// Hawtconfig defines the hawtconfig.json structure
type Hawtconfig struct {
	Branding struct {
		AppName     string `json:"appName"`
		AppLogoURL  string `json:"appLogoUrl"`
		ConsoleLink struct {
			Text              string `json:"text"`
			Section           string `json:"section"`
			ImageRelativePath string `json:"imageRelativePath"`
		} `json:"consoleLink"`
	} `json:"branding"`
}

func init() {
	SchemeBuilder.Register(&Hawtio{}, &HawtioList{})
}
