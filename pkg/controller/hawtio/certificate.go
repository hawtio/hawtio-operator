package hawtio

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	rand2 "math/rand"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func generateSelfSignedCertSecret(name string, namespace string, commonName string, expirationDate time.Time) (*corev1.Secret, error) {
	return generateCertificateSecret(name, namespace, nil, commonName, expirationDate)
}

func generateCASignedCertSecret(name string, namespace string, caSecret *corev1.Secret, commonName string, expirationDate time.Time) (*corev1.Secret, error) {
	if caSecret == nil {
		return nil, errors.New("Generating a CA-signed certificate requires the CA Secret")
	}

	return generateCertificateSecret(name, namespace, caSecret, commonName, expirationDate)
}

func generateCertificateSecret(name string, namespace string, caSecret *corev1.Secret, commonName string, expirationDate time.Time) (*corev1.Secret, error) {
	var caCert *x509.Certificate
	var caPrivateKey crypto.PrivateKey
	var err error

	if caSecret != nil {
		caCertFile := caSecret.Data["tls.crt"]
		pemBlock, _ := pem.Decode(caCertFile)
		if pemBlock == nil {
			return nil, errors.New("failed to decode CA certificate")
		}
		caCert, err = x509.ParseCertificate(pemBlock.Bytes)
		if err != nil {
			return nil, err
		}

		caKey := caSecret.Data["tls.key"]
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

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		}, Data: map[string][]byte{
			corev1.TLSCertKey:       certPEM,
			corev1.TLSPrivateKeyKey: privateKeyPem,
		}, Type: corev1.SecretTypeTLS,
	}, nil
}

func ValidateCertificate(caSecret corev1.Secret, validAtLeastForHours float64) (bool, error) {
	block, _ := pem.Decode(caSecret.Data[corev1.TLSCertKey])
	cert, err := x509.ParseCertificate(block.Bytes)

	if err != nil {
		log.Error(err, "certificate reading error")
		return false, err
	}

	diff := cert.NotAfter.Sub(time.Now()).Hours()
	// if cert is valid longer than certain amount of hours
	if diff > validAtLeastForHours {
		log.Info(fmt.Sprintf("Certificate is valid for %.0f days", diff/24))
		return true, nil
	}
	//if is valid
	return false, nil
}
