package util

import (
	"fmt"
	"os"
)

// DebugLogLevel is an alias to use in log.V(...)
// See Increasing Verbosity in
// https://pkg.go.dev/github.com/go-logr/zapr#section-readme
const DebugLogLevel = 1

// Go build-time variables
type BuildVariables struct {
	// The hawtio-online operand image repository
	ImageRepository string
	// The hawtio-online-gateway operand image repository
	GatewayImageRepository string
	// The hawtio-online operand image version
	ImageVersion string
	// The hawtio-gateway operand image version
	GatewayImageVersion string
	// The operator version
	OperatorVersion string
	// Legacy serving certificate version
	LegacyServingCertificateMountVersion string
	// Product name
	ProductName string
	// The hawtio-online server root directory
	ServerRootDirectory string
	// The default common name for the generated client certificates
	ClientCertCommonName string
	// Additional template spec labels
	AdditionalLabels string
}

// GetOnlineVersion returns the preferred online version
// taking into account the IMAGE_VERSION env var and the
// ImageVersion property
func (bv *BuildVariables) GetOnlineVersion() string {
	fmt.Println("Getting version from IMAGE_VERSION environment variable ...")
	version := os.Getenv("IMAGE_VERSION")
	if version == "" {
		fmt.Println("Getting version from build variable ImageVersion")
		version = bv.ImageVersion
		if len(version) == 0 {
			fmt.Println("Defaulting to version being latest")
			version = "latest"
		}
	}
	return version
}

// GetGatewayVersion returns the preferred gateway version
// taking into account the GATEWAY_IMAGE_VERSION env var and the
// GatewayImageVersion property
func (bv *BuildVariables) GetGatewayVersion() string {
	fmt.Println("Getting version from GATEWAY_IMAGE_VERSION environment variable ...")
	version := os.Getenv("GATEWAY_IMAGE_VERSION")
	if version == "" {
		fmt.Println("Getting version from build variable GatewayImageVersion")
		version = bv.GatewayImageVersion
		if len(version) == 0 {
			fmt.Println("Defaulting to online version")
			version = bv.GetOnlineVersion()
		}
	}
	return version
}
