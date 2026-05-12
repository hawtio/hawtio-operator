package hawtio

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	rand2 "math/rand"
	"time"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/resources"
)

func generateSelfSignedCertSecret(hawtio *hawtiov2.Hawtio, name string, namespace string, commonName string, expirationDate time.Time) (*corev1.Secret, error) {
	return generateCertificateSecret(hawtio, name, namespace, nil, commonName, expirationDate)
}

func generateCASignedCertSecret(hawtio *hawtiov2.Hawtio, name string, namespace string, caSecret *corev1.Secret, commonName string, expirationDate time.Time) (*corev1.Secret, error) {
	if caSecret == nil {
		return nil, errors.New("Generating a CA-signed certificate requires the CA Secret")
	}

	return generateCertificateSecret(hawtio, name, namespace, caSecret, commonName, expirationDate)
}

func generateCertificateSecret(hawtio *hawtiov2.Hawtio, name string, namespace string, caSecret *corev1.Secret, commonName string, expirationDate time.Time) (*corev1.Secret, error) {
	var caCert *x509.Certificate
	var caPrivateKey crypto.PrivateKey
	var err error

	if caSecret != nil {
		caCertFile := caSecret.Data[corev1.TLSCertKey]
		pemBlock, _ := pem.Decode(caCertFile)
		if pemBlock == nil {
			return nil, errors.New("failed to decode CA certificate")
		}
		caCert, err = x509.ParseCertificate(pemBlock.Bytes)
		if err != nil {
			return nil, err
		}

		caKey := caSecret.Data[corev1.TLSPrivateKeyKey]
		pemBlock, _ = pem.Decode(caKey)
		if pemBlock == nil {
			return nil, errors.New("failed to decode CA certificate signing key")
		}
		caPrivateKey, err = x509.ParsePKCS1PrivateKey(pemBlock.Bytes)
		if err != nil {
			return nil, err
		}
	}

	serialNumber := big.NewInt(rand2.Int63())
	cert := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:   time.Now(),
		NotAfter:    expirationDate,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	if caCert == nil {
		// No CA certificate provided so create self-signed certificate
		caCert = cert
	}

	// generate cert private key
	certPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	if caPrivateKey == nil {
		// No CA certificate provided so create self-signed certificate
		caPrivateKey = certPrivateKey
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(certPrivateKey)
	// encode for storing into secret
	privateKeyPem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: privateKeyBytes,
		},
	)
	certBytes, err := x509.CreateCertificate(rand.Reader, cert, caCert, &certPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		return nil, err
	}
	// encode for storing into secret
	certPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certBytes,
		})

	labels := resources.LabelsForHawtio(hawtio.Name)
	resources.PropagateLabels(hawtio, labels, hawtioLogger)

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		}, Data: map[string][]byte{
			corev1.TLSCertKey:       certPEM,
			corev1.TLSPrivateKeyKey: privateKeyPem,
		}, Type: corev1.SecretTypeTLS,
	}, nil
}

func certificateExpiryPeriod(hawtio *hawtiov2.Hawtio) time.Duration {
	periodHours := hawtio.Spec.Auth.ClientCertExpirationPeriod
	if periodHours == 0 {
		periodHours = 24
	}

	return time.Duration(periodHours)
}

// checkCertificateExpiry evaluates the client certificate.
// Returns: (nextCheck time.Duration) where 0 is rotate immediately
func checkCertificateExpiry(hawtio *hawtiov2.Hawtio, secret *corev1.Secret, log logr.Logger) time.Duration {
	certData, exists := secret.Data[corev1.TLSCertKey]
	if !exists {
		return 0 // Malformed secret, overwrite it
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return 0
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return 0
	}

	periodHours := certificateExpiryPeriod(hawtio)
	threshold := periodHours * time.Hour
	timeUntilExpiry := time.Until(cert.NotAfter)
	if timeUntilExpiry <= threshold {
		log.Info("Certificate expired or expiring soon. In-place rotation required.")
		return 0
	}

	sleepDuration := timeUntilExpiry - threshold
	return sleepDuration
}
