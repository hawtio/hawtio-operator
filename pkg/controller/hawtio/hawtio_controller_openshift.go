package hawtio

import (
	"context"
	"fmt"
	"os"
	"time"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/resources"
	"github.com/hawtio/hawtio-operator/pkg/util"

	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var conOsLog = logf.Log.WithName("controller_hawtio_openshift")

// newSignedCertificateSecret create a certificate signed by the openshift
// certificate authority key.
// Sets the validity according to the operator's own environment variable
// CERTIFICATE_EXPIRY_PERIOD.
func newSignedCertificateSecret(ctx context.Context, r *ReconcileHawtio, hawtio *hawtiov2.Hawtio, name string, namespace string) (*corev1.Secret, error) {
	caSecret, err := r.coreClient.Secrets("openshift-service-ca").Get(ctx, "signing-key", metav1.GetOptions{})
	if err != nil {
		return nil, errs.Wrap(err, "Reading certificate authority signing key failed")
	}

	// expiration determined by today + CERTIFICATE_EXPIRY_PERIOD
	expirationDate := time.Now().Add(r.certExpiryPeriod)
	clientCertSecret, err := generateCASignedCertSecret(hawtio, name, namespace, caSecret, HAWTIO_CERT_COMMON_NAME, expirationDate)
	if err != nil {
		return nil, errs.Wrap(err, "Generating the client certificate failed")
	}

	conOsLog.V(util.DebugLogLevel).Info("Created Signed Certificate", "secret", clientCertSecret.Name, "expires", expirationDate.String())
	return clientCertSecret, nil
}

// If we're in test mode, don't try to create a real cert.
// Just log it and return 'nil' to signal "no error, nothing to do".
func newCertificateSecret(ctx context.Context, r *ReconcileHawtio, hawtio *hawtiov2.Hawtio, clientSecretName string, namespace string) (*corev1.Secret, error) {
	var clientCertSecret *corev1.Secret
	var err error
	if os.Getenv(util.HawtioUnderTestEnvVar) == "true" {
		conOsLog.Info(fmt.Sprintf("%s: Creating OpenShift self-signed mock proxying certificate", util.HawtioUnderTestEnvVar))
		clientCertSecret, err = newSelfCertificateSecret(ctx, r, hawtio, clientSecretName, namespace)
	} else {
		clientCertSecret, err = newSignedCertificateSecret(ctx, r, hawtio, clientSecretName, namespace)
	}

	if err != nil {
		return nil, err
	}

	return clientCertSecret, nil
}

func (r *ReconcileHawtio) osCreateClientCertificate(ctx context.Context, hawtio *hawtiov2.Hawtio, clientSecretName string) (*corev1.Secret, time.Duration, error) {
	// Secret should be in the operator's own namespace
	namespace := r.operatorPod.Namespace

	// Check whether client certificate secret exists in operator namespace
	clientCertSecret, err := r.coreClient.Secrets(namespace).Get(ctx, clientSecretName, metav1.GetOptions{})
	if err == nil {
		// Found the secret

		// Check the secret's labels
		labels := clientCertSecret.GetLabels()
		if labels == nil || labels[resources.LabelAppKey] != resources.LabelAppValue {
			conOsLog.V(util.DebugLogLevel).Info("Adopting existing master client certificate", "namespace", r.operatorPod.Namespace, "secret", clientSecretName)
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
		nextCheckIn := checkCertificateExpiry(r.certExpiryPeriod, clientCertSecret, conOsLog)
		conOsLog.V(util.DebugLogLevel).Info("Master client certificate (existing) validity", "next check-in", nextCheckIn.String())
		if nextCheckIn == 0 {
			// certificate is invalid or close to expiring
			// create a new one and update the secret
			newSecret, err := newCertificateSecret(ctx, r, hawtio, clientCertSecret.Name, clientCertSecret.Namespace)
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

			// reset nextCheckIn to maximum as new certificate
			nextCheckIn = checkCertificateExpiry(r.certExpiryPeriod, clientCertSecret, conOsLog)
			conOsLog.V(util.DebugLogLevel).Info("Master client certificate (update) check-in reset", "next check-in", nextCheckIn.String())
		}

		return clientCertSecret, nextCheckIn, nil
	}

	if kerrors.IsNotFound(err) {
		conOsLog.Info("Client certificate secret not found, creating a new one", "secret", clientSecretName)

		var clientCertSecret *corev1.Secret
		clientCertSecret, err := newCertificateSecret(ctx, r, hawtio, clientSecretName, namespace)
		if err != nil {
			return nil, 0, err
		}

		clientCertSecret, err = r.coreClient.Secrets(namespace).Create(ctx, clientCertSecret, metav1.CreateOptions{})
		conOsLog.Info("Client certificate created successfully", "secret", clientSecretName, "Resource Version", clientCertSecret.GetResourceVersion())
		if err != nil {
			return nil, 0, errs.Wrap(err, "Creating the client certificate secret failed")
		}

		// New Secret so maximum expiry period with buffer
		nextCheckIn := checkCertificateExpiry(r.certExpiryPeriod, clientCertSecret, conOsLog)
		conOsLog.V(util.DebugLogLevel).Info("Master client certificate (new) check-in reset", "next check-in", nextCheckIn.String())
		return clientCertSecret, nextCheckIn, nil
	}

	// error was something but not NotFound
	return nil, 0, err
}
