package common

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MetadataInfo provides properties related to the resource metadata
type MetadataInfo struct {
	Name               string
	Namespace          string
	LabelPatterns      []string
	AnnotationPatterns []string
}

// CertificateOptions provides properties related to certificates
type CertificateOptions struct {
	CommonName       string
	ExpirationDate   *metav1.Time
	ExpirationPeriod int
}

// RbacOptions provides properties related to RBAC settings
type RbacOptions struct {
	ConfigMap           string
	DisableRBACRegistry *bool
}

// NginxOptions provides properties for Nginx tuning settings
type NginxOptions struct {
	ClientBodyBufferSize       string
	ProxyBuffers               string
	SubrequestOutputBufferSize string
	MasterBurstSize            string
}
