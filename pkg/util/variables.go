package util

// Go build-time variables
type BuildVariables struct {
	// The hawtio-online operand image repository
	ImageRepository string
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
