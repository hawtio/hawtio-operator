package util

import (
	"encoding/json"
	"fmt"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
)

// IsSSL Determine if deployment should be via SSL or not
// SSL should be switched on for OpenShift 4+
// For backward compatibility if there is no SSL Prop then default to true
func IsSSL(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec) bool {
	sslLogMsg := "Should deployment use SSL: "

	if apiSpec.IsOpenShift4 {
		sslLogMsg = fmt.Sprintf("%s true [Using OpenShift]\n", sslLogMsg)
		return true // Always on for OpenShift 4+
	}

	if hawtio.Spec.Auth.InternalSSL == nil {
		sslLogMsg = fmt.Sprintf("%s true [InternalSSL not defined]\n", sslLogMsg)
		return true // Should be switched on by default
	}

	log.V(DebugLogLevel).Info(fmt.Sprintf("%s %t [Value of InternalSSL in CR]\n", sslLogMsg, *hawtio.Spec.Auth.InternalSSL))
	return *hawtio.Spec.Auth.InternalSSL
}

// JSONToString Convert an object to a json string
func JSONToString(v interface{}) string {
	j, err := json.Marshal(v)
	if err != nil {
		log.Error(err, "Cannot convert to json")
		return "{}"
	}

	return string(j)
}
