package util

import (
	"fmt"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
)

// IsSSL Determine if deployment should be via SSL or not
// SSL should be switched on for OpenShift 4+
// For backward compatibility if there is no SSL Prop then default to true
func IsSSL(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec) bool {
	fmt.Print("Should deployment use SSL: ")

	if apiSpec.IsOpenShift4 {
		fmt.Print("true [Using OpenShift]\n")
		return true // Always on for OpenShift 4+
	}

	if hawtio.Spec.Auth.InternalSSL == nil {
		fmt.Print("true [InternalSSL not defined]\n")
		return true // Should be switched on by default
	}

	fmt.Printf("%t [Value of InternalSSL in CR]\n", *hawtio.Spec.Auth.InternalSSL)
	return *hawtio.Spec.Auth.InternalSSL
}
