package constants

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	// CurrentVersion product version supported
	CurrentVersion = "1.6"
	// PriorVersion1 product version supported
	PriorVersion1 = "1.5"
	// PriorVersion2 product version supported
	PriorVersion2 = "1.4"

	RedHatImageRegistry = "registry.redhat.io"

	HawtioVar                   = "HAWTIO_IMAGE_"
	HawtioImage               = "fuse-console"

	ImageURLCurrentVersion        = RedHatImageRegistry + "/fuse7/" + HawtioImage + ":" + CurrentVersion
	ImageURLPriorVersion1         = RedHatImageRegistry + "/fuse7/" + HawtioImage + ":" + PriorVersion1
	ImageURLPriorVersion2         = RedHatImageRegistry + "/fuse7/" + HawtioImage + ":" + PriorVersion2



	HawtioImageTagComponent = "fuse-console-openshift-container"
)

// SupportedVersions - product versions this operator supports
var SupportedVersions = []string{CurrentVersion, PriorVersion1, PriorVersion2}

// VersionConstants ...
var VersionConstants = map[string]VersionConfigs{
	CurrentVersion: {

		APIVersion: api.SchemeGroupVersion.Version,
		HawtioImage: 		HawtioImage,
		HawtioImageTag:     CurrentVersion,
		HawtioImageURL:     ImageURLCurrentVersion,
	},
	PriorVersion1:  {
		APIVersion: api.SchemeGroupVersion.Version,
		HawtioImage: 		HawtioImage,
		HawtioImageTag:     PriorVersion1,
		HawtioImageURL:     ImageURLPriorVersion1,

	},

	PriorVersion2: {

		APIVersion: api.SchemeGroupVersion.Version,
		HawtioImage: 		HawtioImage,
		HawtioImageTag:     PriorVersion2,
		HawtioImageURL:     ImageURLPriorVersion2,



	},
}

// VersionConfigs ...
type VersionConfigs struct {
	APIVersion      string `json:"apiVersion,omitempty"`
	HawtioImage     string `json:"hawtioImage,omitempty"`
	HawtioImageTag  string `json:"hawtioImageTag,omitempty"`
	HawtioImageURL  string `json:"hawtioImageURL,omitempty"`
	HawtioComponent string `json:"hawtioComponent,omitempty"`
}

type ImageEnv struct {
	Var       string
	Component string
	Registry  string
}
type ImageRef struct {
	metav1.TypeMeta `json:",inline"`
	Spec            ImageRefSpec `json:"spec"`
}
type ImageRefSpec struct {
	Tags []ImageRefTag `json:"tags"`
}
type ImageRefTag struct {
	Name string                  `json:"name"`
	From *corev1.ObjectReference `json:"from"`
}
