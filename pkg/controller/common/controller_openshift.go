package common

import (
	"context"
	"time"

	commonapi "github.com/hawtio/hawtio-operator/pkg/apis/common"
	"github.com/hawtio/hawtio-operator/pkg/util"

	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	resources "github.com/hawtio/hawtio-operator/pkg/resources/common"
)

func OsCreateClientCertificate(ctx context.Context, r *ReconcileBase, hawtio commonapi.HawtioGeneral, clientSecretName string) (*corev1.Secret, time.Duration, error) {
	// Check whether client certificate secret exists in operator namespace
	namespace := r.OperatorPod.Namespace
	clientCertSecret, err := r.CoreClient.Secrets(namespace).Get(ctx, clientSecretName, metav1.GetOptions{})
	if err == nil {
		// Found the secret

		// Check the secret's labels
		labels := clientCertSecret.GetLabels()
		if labels == nil || labels[resources.LabelAppKey] != resources.LabelAppValue {
			r.Logger.V(util.DebugLogLevel).Info("Adopting existing master client certificate", "namespace", namespace, "secret", clientSecretName)
			// This a legacy certificate so adopt it
			// Note: adoptLegacyResource returns the Sentinel Error (ErrLegacyResourceAdopted)
			// on success.
			adoptErr := r.AdoptLegacyResource(ctx, clientCertSecret)
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
		nextCheckIn := checkCertificateExpiry(r.CertExpiryPeriod, clientCertSecret, r.Logger)
		r.Logger.V(util.DebugLogLevel).Info("Master client certificate (existing) validity", "next check-in", nextCheckIn.String())
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
			if err := r.Client.Update(ctx, clientCertSecret); err != nil {
				return nil, 0, err
			}

			// reset nextCheckIn to maximum as new certificate
			nextCheckIn = checkCertificateExpiry(r.CertExpiryPeriod, clientCertSecret, r.Logger)
			r.Logger.V(util.DebugLogLevel).Info("Master client certificate (update) check-in reset", "next check-in", nextCheckIn.String())
		}

		return clientCertSecret, nextCheckIn, nil
	}

	if kerrors.IsNotFound(err) {
		r.Logger.Info("Client certificate secret not found, creating a new one", "secret", clientSecretName)

		var clientCertSecret *corev1.Secret
		clientCertSecret, err := newCertificateSecret(ctx, r, hawtio, clientSecretName, namespace)
		if err != nil {
			return nil, 0, err
		}

		clientCertSecret, err = r.CoreClient.Secrets(namespace).Create(ctx, clientCertSecret, metav1.CreateOptions{})
		r.Logger.Info("Client certificate created successfully", "secret", clientSecretName, "Resource Version", clientCertSecret.GetResourceVersion())
		if err != nil {
			return nil, 0, errs.Wrap(err, "Creating the client certificate secret failed")
		}

		// New Secret so maximum expiry period with buffer
		nextCheckIn := checkCertificateExpiry(r.CertExpiryPeriod, clientCertSecret, r.Logger)
		r.Logger.V(util.DebugLogLevel).Info("Master client certificate (new) check-in reset", "next check-in", nextCheckIn.String())
		return clientCertSecret, nextCheckIn, nil
	}

	// error was something but not NotFound
	return nil, 0, err
}
