package util

// Go build-time variables
type BuildVariables struct {
	// The hawtio-online operand image repository
	ImageRepository string
	// Legacy serving certificate version
	LegacyServingCertificateMountVersion string
	// Product name
	ProductName string
}
