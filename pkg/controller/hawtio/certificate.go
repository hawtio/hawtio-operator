package hawtio

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	errors2 "errors"
	"math/big"
	rand2 "math/rand"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func generateCertificateSecret(name string, namespace string, caSecret *corev1.Secret, commonName string, daysToExpiry int) (*corev1.Secret, error) {
	caCertFile := caSecret.Data["tls.crt"]
	pemBlock, _ := pem.Decode(caCertFile)
	if pemBlock == nil {
		return nil, errors2.New("failed to decode CA certificate")
	}
	caCert, err := x509.ParseCertificate(pemBlock.Bytes)
	if err != nil {
		return nil, err
	}

	caKey := caSecret.Data["tls.key"]
	pemBlock, _ = pem.Decode(caKey)
	if pemBlock == nil {
		return nil, errors2.New("failed to decode CA certificate signing key")
	}
	caPrivateKey, err := x509.ParsePKCS1PrivateKey(pemBlock.Bytes)
	if err != nil {
		return nil, err
	}

	serialNumber := big.NewInt(rand2.Int63())
	cert := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(0, 0, daysToExpiry),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	// generate cert private key
	certPrivetKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(certPrivetKey)
	// encode for storing into secret
	privateKeyPem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: privateKeyBytes,
		},
	)
	certBytes, err := x509.CreateCertificate(rand.Reader, cert, caCert, &certPrivetKey.PublicKey, caPrivateKey)
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
