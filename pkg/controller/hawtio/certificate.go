package hawtio

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	v1 "k8s.io/api/batch/v1"
	"k8s.io/api/batch/v1beta1"
	"math/big"
	rand2 "math/rand"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func generateCertificateSecret(name string, namespace string, caSecret *corev1.Secret, commonName string, expirationDate time.Time) (*corev1.Secret, error) {
	caCertFile := caSecret.Data["tls.crt"]
	pemBlock, _ := pem.Decode(caCertFile)
	if pemBlock == nil {
		return nil, errors.New("failed to decode CA certificate")
	}
	caCert, err := x509.ParseCertificate(pemBlock.Bytes)
	if err != nil {
		return nil, err
	}

	caKey := caSecret.Data["tls.key"]
	pemBlock, _ = pem.Decode(caKey)
	if pemBlock == nil {
		return nil, errors.New("failed to decode CA certificate signing key")
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
		NotAfter:    expirationDate,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	// generate cert private key
	certPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
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

func createCertValidationCronJob(name, namespace, image, schedule string, period int) *v1beta1.CronJob {
	if period == 0 {
		period = 24
	}
	cronjob := &v1beta1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1beta1.CronJobSpec{
			Schedule:          schedule,
			ConcurrencyPolicy: v1beta1.ForbidConcurrent,
			JobTemplate: v1beta1.JobTemplateSpec{
				Spec: v1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: "hawtio-operator",
							RestartPolicy:      "Never",
							Containers: []corev1.Container{
								{
									Name:  "hawtio-operator",
									Image: image,
									Command: []string{
										"hawtio-operator",
									},
									Args: []string{
										"cert-expiry-check",
										"--cert-namespace",
										namespace,
										"--cert-expiration-period",
										strconv.Itoa(period),
									},
									ImagePullPolicy: "Always",
								},
							},
						},
					},
				},
			},
		},
	}
	return cronjob
}

func updateExpirationPeriod(cronJob *v1beta1.CronJob, newPeriod int) bool {
	arguments := cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Args
	for i, arg := range arguments {
		if arg == "--cert-expiration-period" {
			period, _ := strconv.Atoi(arguments[i+1])
			if period == newPeriod {
				return false
			}
			cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Args[i+1] = strconv.Itoa(newPeriod)
			return true
		}
	}
	return false
}
