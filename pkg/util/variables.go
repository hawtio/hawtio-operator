package util

import (
	"fmt"
	"os"
	"strings"
)

// DebugLogLevel is an alias to use in log.V(...)
// See Increasing Verbosity in
// https://pkg.go.dev/github.com/go-logr/zapr#section-readme
const DebugLogLevel = 1

const (
	// ImageRepositoryEnvVar is the IMAGE_REPOSITORY env var
	ImageRepositoryEnvVar = "IMAGE_REPOSITORY"
	// ImageRepositoryEnvVarDefault default value for IMAGE_REPOSITORY
	ImageRepositoryEnvVarDefault = "quay.io/hawtio/online"
	// GatewayImageRepositoryEnvVar is the GATEWAY_IMAGE_REPOSITORY env var
	GatewayImageRepositoryEnvVar = "GATEWAY_IMAGE_REPOSITORY"
	// GatewayImageRepositoryEnvVarDefault default value for GATEWAY_IMAGE_REPOSITORY
	GatewayImageRepositoryEnvVarDefault = "quay.io/hawtio/online-gateway"

	// PluginImageRepoEnvVar is the PLUGIN_IMAGE_REPOSITORY env var
	PluginImageRepoEnvVar = "PLUGIN_IMAGE_REPOSITORY"
	// PluginImageRepoEnvVarDefault default value for IMAGE_REPOSITORY
	PluginImageRepoEnvVarDefault = "quay.io/hawtio/online-console-plugin"
	// PluginGatewayImageRepoEnvVar is the GATEWAY_IMAGE_REPOSITORY env var
	PluginGatewayImageRepoEnvVar = "PLUGIN_GATEWAY_IMAGE_REPOSITORY"
	// PluginGatewayImageRepoEnvVarDefault default value for GATEWAY_IMAGE_REPOSITORY
	PluginGatewayImageRepoEnvVarDefault = "quay.io/hawtio/online-console-plugin-gateway"

	// ImageVersionEnvVar is the IMAGE_VERSION env var
	ImageVersionEnvVar = "IMAGE_VERSION"
	// ImageVersionEnvVarDefault default value for IMAGE_VERSION
	ImageVersionEnvVarDefault = "latest"
	// GatewayImageVersionEnvVar is the GATEWAY_IMAGE_VERSION env var
	GatewayImageVersionEnvVar = "GATEWAY_IMAGE_VERSION"
	// GatewayImageVersionEnvVarDefault default value for GATEWAY_IMAGE_VERSION
	GatewayImageVersionEnvVarDefault = "latest"

	// PluginImageVersionEnvVar is the PLUGIN_IMAGE_VERSION env var
	PluginImageVersionEnvVar = "PLUGIN_IMAGE_VERSION"
	// PluginImageVersionEnvVarDefault default value for PLUGIN_IMAGE_VERSION
	PluginImageVersionEnvVarDefault = "latest"
	// PluginGatewayImageVersionEnvVar is the PLUGIN_GATEWAY_IMAGE_VERSION env var
	PluginGatewayImageVersionEnvVar = "PLUGIN_GATEWAY_IMAGE_VERSION"
	// PluginGatewayImageVersionEnvVarDefault default value for PLUGIN_GATEWAY_IMAGE_VERSION
	PluginGatewayImageVersionEnvVarDefault = "latest"
)

type HawtioPluginVars struct {
	// The hawtio plugin operand image repository
	ImageRepository string
	// The hawtio plugin gateway operand image repository
	GatewayImageRepository string
	// The hawtio plugin operand image version
	ImageVersion string
	// The hawtio plugin gateway operand image version
	GatewayImageVersion string
}

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
	// HawtioPlugin build-time properties
	Plugin HawtioPluginVars
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

// ImageURI Provides a sha or tag URI for an image
func (bv *BuildVariables) ImageURI(repository string, tag string) string {
	if strings.HasPrefix(tag, "sha256:") {
		// tag is a sha checksum tag
		return repository + "@" + tag
	}

	return repository + ":" + tag
}

func getEnvVarOrDefault(envVar string, buildVarValue string, defaultValue string) string {
	fmt.Printf("Getting value from %s environment variable ...\n", envVar)
	v := os.Getenv(envVar)
	if v == "" {
		fmt.Printf("Getting value from build variable %s\n", buildVarValue)
		v = buildVarValue
		if len(v) == 0 {
			fmt.Printf("Falling back to default value %s\n", defaultValue)
			v = defaultValue
		}
	}
	return v
}

// GetOnlineImageRepo returns the preferred online image repository
// taking into account the IMAGE_REPOSITORY env var
func (bv *BuildVariables) GetOnlineImageRepo() string {
	return getEnvVarOrDefault(ImageRepositoryEnvVar, bv.ImageRepository, ImageRepositoryEnvVarDefault)
}

// GetOnlineGatewayImageRepo returns the preferred online image repository
// taking into account the GATEWAY_IMAGE_REPOSITORY env var
func (bv *BuildVariables) GetOnlineGatewayImageRepo() string {
	return getEnvVarOrDefault(GatewayImageRepositoryEnvVar, bv.GatewayImageRepository, GatewayImageRepositoryEnvVarDefault)
}

// GetOnlineVersion returns the preferred online version
// taking into account the IMAGE_VERSION env var and the
// ImageVersion property
func (bv *BuildVariables) GetOnlineVersion() string {
	return getEnvVarOrDefault(ImageVersionEnvVar, bv.ImageVersion, ImageVersionEnvVarDefault)
}

// GetOnlineGatewayVersion returns the preferred gateway version
// taking into account the GATEWAY_IMAGE_VERSION env var and the
// GatewayImageVersion property
func (bv *BuildVariables) GetOnlineGatewayVersion() string {
	return getEnvVarOrDefault(GatewayImageVersionEnvVar, bv.GatewayImageVersion, GatewayImageVersionEnvVarDefault)
}

// GetPluginImageRepo returns the preferred plugin image repository
// taking into account the PLUGIN_IMAGE_REPOSITORY env var
func (bv *BuildVariables) GetPluginImageRepo() string {
	return getEnvVarOrDefault(PluginImageRepoEnvVar, bv.Plugin.ImageRepository, PluginImageRepoEnvVarDefault)
}

// GetPluginGatewayImageRepo returns the preferred plugin gateway image repository
// taking into account the PLUGIN_GATEWAY_IMAGE_REPOSITORY env var
func (bv *BuildVariables) GetPluginGatewayImageRepo() string {
	return getEnvVarOrDefault(PluginGatewayImageRepoEnvVar, bv.Plugin.GatewayImageRepository, PluginGatewayImageRepoEnvVarDefault)
}

// GetPluginVersion returns the preferred online-console-plugin version
// taking into account the PLUGIN_IMAGE_VERSION env var and the
// ImageVersion property
func (bv *BuildVariables) GetPluginVersion() string {
	return getEnvVarOrDefault(PluginImageVersionEnvVar, bv.Plugin.ImageVersion, PluginImageVersionEnvVarDefault)
}

// GetPluginGatewayVersion returns the preferred online-console-plugin-gateway version
// taking into account the PLUGIN_GATEWAY_IMAGE_VERSION env var and the
// GatewayImageVersion property
func (bv *BuildVariables) GetPluginGatewayVersion() string {
	return getEnvVarOrDefault(PluginGatewayImageVersionEnvVar, bv.Plugin.GatewayImageVersion, PluginGatewayImageVersionEnvVarDefault)
}
