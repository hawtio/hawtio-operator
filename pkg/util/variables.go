package util

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
