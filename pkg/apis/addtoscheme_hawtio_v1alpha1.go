package apis

import (
	"github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	"github.com/hawtio/hawtio-operator/pkg/openshift"
	consolev1 "github.com/openshift/api/console/v1"
)

func init() {
	// Register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(AddToSchemes, v1alpha1.SchemeBuilder.AddToScheme)
	if err := openshift.ConsoleYAMLSampleExists(); err == nil {
		AddToSchemes = append(AddToSchemes, consolev1.Install)
	}
}
