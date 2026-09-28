package common

import (
	"context"
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
	"os"
	"time"

	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonapi "github.com/hawtio/hawtio-operator/pkg/apis/common"
	"github.com/hawtio/hawtio-operator/pkg/resources"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

const HAWTIO_CERT_COMMON_NAME = "hawtio-online.hawtio.svc"

func generateCertificateSecret(hawtio commonapi.HawtioGeneral, name string, namespace string, caSecret *corev1.Secret, commonName string, expirationDate time.Time, logger logr.Logger) (*corev1.Secret, error) {
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
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
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

	labels := resources.LabelsForHawtio(hawtio.GetName())
	resources.PropagateLabels(hawtio, labels, logger)

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

func generateSelfSignedCertSecret(hawtio commonapi.HawtioGeneral, name string, namespace string, commonName string, expirationDate time.Time, logger logr.Logger) (*corev1.Secret, error) {
	return generateCertificateSecret(hawtio, name, namespace, nil, commonName, expirationDate, logger)
}

func generateCASignedCertSecret(hawtio commonapi.HawtioGeneral, name string, namespace string, caSecret *corev1.Secret, commonName string, expirationDate time.Time, logger logr.Logger) (*corev1.Secret, error) {
	if caSecret == nil {
		return nil, errors.New("Generating a CA-signed certificate requires the CA Secret")
	}

	return generateCertificateSecret(hawtio, name, namespace, caSecret, commonName, expirationDate, logger)
}

func newSignedCertificateSecret(ctx context.Context, r *ReconcileBase, hawtio commonapi.HawtioGeneral, name string, namespace string) (*corev1.Secret, error) {
	caSecret, err := r.CoreClient.Secrets("openshift-service-ca").Get(ctx, "signing-key", metav1.GetOptions{})
	if err != nil {
		return nil, errs.Wrap(err, "Reading certificate authority signing key failed")
	}

	// expiration determined by today + CERTIFICATE_EXPIRY_PERIOD
	expirationDate := time.Now().Add(r.CertExpiryPeriod)
	clientCertSecret, err := generateCASignedCertSecret(hawtio, name, namespace, caSecret, HAWTIO_CERT_COMMON_NAME, expirationDate, r.Logger)
	if err != nil {
		return nil, errs.Wrap(err, "Generating the client certificate failed")
	}

	r.Logger.V(util.DebugLogLevel).Info("Created Signed Certificate", "secret", clientCertSecret.Name, "expires", expirationDate.String())
	return clientCertSecret, nil
}

func newSelfCertificateSecret(ctx context.Context, r *ReconcileBase, hawtio commonapi.HawtioGeneral, name string, namespace string) (*corev1.Secret, error) {
	// Let's default to one year validity period
	expirationDate := time.Now().AddDate(1, 0, 0)
	if date := hawtio.GetCertificateOptions().ExpirationDate; date != nil && !date.IsZero() {
		expirationDate = date.Time
	}
	servingCertSecret, err := generateSelfSignedCertSecret(hawtio, name, namespace, HAWTIO_CERT_COMMON_NAME, expirationDate, r.Logger)
	if err != nil {
		return nil, errs.Wrap(err, "Generating the serving certificate failed")
	}

	return servingCertSecret, nil
}

// If we're in test mode, don't try to create a real cert.
// Just log it and return 'nil' to signal "no error, nothing to do".
func newCertificateSecret(ctx context.Context, r *ReconcileBase, hawtio commonapi.HawtioGeneral, clientSecretName string, namespace string) (*corev1.Secret, error) {
	var clientCertSecret *corev1.Secret
	var err error
	if os.Getenv(util.HawtioUnderTestEnvVar) == "true" {
		r.Logger.Info(fmt.Sprintf("%s: Creating OpenShift self-signed mock proxying certificate", util.HawtioUnderTestEnvVar))
		clientCertSecret, err = newSelfCertificateSecret(ctx, r, hawtio, clientSecretName, namespace)
	} else {
		clientCertSecret, err = newSignedCertificateSecret(ctx, r, hawtio, clientSecretName, namespace)
	}

	if err != nil {
		return nil, err
	}

	return clientCertSecret, nil
}

// calculateRequeueBuffer calculates the buffer
// as a proportion of the total duration
func calculateRequeueBuffer(totalDuration time.Duration) time.Duration {
	// Buffer is 10% of total lifetime, up to a max of 1 hour
	buffer := totalDuration / 10
	if buffer > (1 * time.Hour) {
		return 1 * time.Hour
	}
	// Floor buffer at 10 seconds for ultra-short test certs
	if buffer < (10 * time.Second) {
		return 10 * time.Second
	}
	return buffer
}

// checkCertificateExpiry evaluates the client certificate.
// Returns: (nextCheck time.Duration) where 0 is rotate immediately
func checkCertificateExpiry(certExpiryPeriod time.Duration, secret *corev1.Secret, logger logr.Logger) time.Duration {
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

	timeUntilExpiry := time.Until(cert.NotAfter)
	buffer := calculateRequeueBuffer(certExpiryPeriod)
	logger.V(util.DebugLogLevel).Info("check certificate expiry",
		"expiry period", certExpiryPeriod.String(),
		"time until expiry", timeUntilExpiry.String(),
		"buffer", buffer.String(),
	)

	// If expired or inside the buffer window, rotate immediately.
	if timeUntilExpiry <= buffer {
		logger.Info("Certificate expired or expiring soon. In-place rotation required.")
		return 0
	}

	// Schedule sleep until the rotation window opens
	sleepDuration := timeUntilExpiry - buffer

	logger.Info("Certificate valid. Scheduling next reconciliation wake-up.", "requeueIn", sleepDuration.String())
	return sleepDuration
}
