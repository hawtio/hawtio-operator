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
	// CommonName required for certificate generation
	CertificateCommonName string
}
