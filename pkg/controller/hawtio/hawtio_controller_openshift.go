package hawtio

import (
	"context"
	"fmt"
	"os"
	"time"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/resources"
	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var conOsLog = logf.Log.WithName("controller_hawtio_openshift")

func newSignedCertificateSecret(ctx context.Context, r *ReconcileHawtio, hawtio *hawtiov2.Hawtio, name string, namespace string) (*corev1.Secret, error) {
	caSecret, err := r.coreClient.Secrets("openshift-service-ca").Get(ctx, "signing-key", metav1.GetOptions{})
	if err != nil {
		return nil, errs.Wrap(err, "Reading certificate authority signing key failed")
	}

	commonName := hawtio.Spec.Auth.ClientCertCommonName
	if commonName == "" {
		if r.ClientCertCommonName == "" {
			commonName = "hawtio-online.hawtio.svc"
		} else {
			commonName = r.ClientCertCommonName
		}
	}
	// Let's default to one year validity period
	expirationDate := time.Now().AddDate(1, 0, 0)
	if date := hawtio.Spec.Auth.ClientCertExpirationDate; date != nil && !date.IsZero() {
		expirationDate = date.Time
	}
	clientCertSecret, err := generateCASignedCertSecret(hawtio, name, namespace, caSecret, commonName, expirationDate)
	if err != nil {
		return nil, errs.Wrap(err, "Generating the client certificate failed")
	}

	return clientCertSecret, nil
}

func osCreateClientCertificate(ctx context.Context, r *ReconcileHawtio, hawtio *hawtiov2.Hawtio) (*corev1.Secret, time.Duration, error) {
	// If we're in test mode, don't try to create a real cert.
	// Just log it and return 'nil' to signal "no error, nothing to do".
	if os.Getenv(HawtioUnderTestEnvVar) == "true" {
		r.logger.Info(fmt.Sprintf("%s: Skipping OpenShift proxying certificate creation", HawtioUnderTestEnvVar))
		return nil, 0, nil
	}

	// This secret name should be the same as used in deployment.go
	clientSecretName := hawtio.Name + "-tls-proxying"

	// Check whether client certificate secret exists
	clientCertSecret, err := r.coreClient.Secrets(hawtio.Namespace).Get(ctx, clientSecretName, metav1.GetOptions{})
	if err == nil {
		// Found the secret

		// Check the secret's labels
		labels := clientCertSecret.GetLabels()
		if labels == nil || labels[resources.LabelAppKey] != resources.LabelAppValue {
			// This a legacy certificate so adopt it
			// Note: adoptLegacyResource returns the Sentinel Error (ErrLegacyResourceAdopted)
			// on success.
			adoptErr := r.adoptLegacyResource(ctx, clientCertSecret)
			if adoptErr != nil {
				// Returns ErrLegacyResourceAdopted (to requeue) or a real API error
				return nil, 0, adoptErr
			}
		}

		//
		// Check the secret's certificate validity.
		// Is the secret certificate invalid (expired).
		// If so they need to update it with a new certificate.
		//
		expiryIn := checkCertificateExpiry(hawtio, clientCertSecret, r.logger)
		if expiryIn == 0 {
			// certificate is invalid or close to expiring
			// create a new one and update the secret
			newSecret, err := newSignedCertificateSecret(ctx, r, hawtio, clientCertSecret.Name, clientCertSecret.Namespace)
			if err != nil {
				return nil, 0, err
			}

			// Initialize the Data map on the existing secret if it's somehow nil
			if clientCertSecret.Data == nil {
				clientCertSecret.Data = make(map[string][]byte)
			}

			// Transplant the fresh crypto material into the existing object
			clientCertSecret.Data[corev1.TLSCertKey] = newSecret.Data[corev1.TLSCertKey]
			clientCertSecret.Data[corev1.TLSPrivateKeyKey] = newSecret.Data[corev1.TLSPrivateKeyKey]

			// Commit the update
			if err := r.client.Update(ctx, clientCertSecret); err != nil {
				return nil, 0, err
			}

			// reset expiryIn to maximum as new certificate
			expiryIn = certificateExpiryPeriod(hawtio)
		}

		return clientCertSecret, expiryIn, nil
	}

	if kerrors.IsNotFound(err) {
		conOsLog.Info("Client certificate secret not found, creating a new one", "secret", clientSecretName)

		clientCertSecret, err := newSignedCertificateSecret(ctx, r, hawtio, clientSecretName, hawtio.Namespace)
		if err != nil {
			return nil, 0, err
		}

		err = controllerutil.SetControllerReference(hawtio, clientCertSecret, r.scheme)
		if err != nil {
			return nil, 0, err
		}
		clientCertSecret, err = r.coreClient.Secrets(hawtio.Namespace).Create(ctx, clientCertSecret, metav1.CreateOptions{})
		conOsLog.Info("Client certificate created successfully", "secret", clientSecretName, "Resource Version", clientCertSecret.GetResourceVersion())
		if err != nil {
			return nil, 0, errs.Wrap(err, "Creating the client certificate secret failed")
		}

		// New Secret so maximum expiry period
		return clientCertSecret, certificateExpiryPeriod(hawtio), nil
	}

	// error was something but not NotFound
	return nil, 0, err
}
