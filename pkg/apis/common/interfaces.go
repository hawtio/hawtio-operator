package common

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HawtioGeneral defines an interface to access
// the common properties of Hawtio and HawtioPlugin
type HawtioGeneral interface {
	// Provides client object properties
	client.Object
	// Core metadata
	GetMetadataInfo() MetadataInfo
	// Certificate Auth Data
	GetCertificateOptions() CertificateOptions
	// Rbac Settings
	GetRbacOptions() RbacOptions
	// Nginx Options
	GetNginxOptions() NginxOptions
}
