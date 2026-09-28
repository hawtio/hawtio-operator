package common

import (
	"context"
	"time"

	commonapi "github.com/hawtio/hawtio-operator/pkg/apis/common"
	"github.com/hawtio/hawtio-operator/pkg/resources"
	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var conKLog = logf.Log.WithName("controller_hawtio_kubernetes")

// getRotationBufferWindow gets the period in hours
// from hawtio CR before expiry of the certificate
func getRotationBufferWindow(hawtio commonapi.HawtioGeneral) time.Duration {
	periodHours := hawtio.GetCertificateOptions().ExpirationPeriod
	if periodHours == 0 {
		periodHours = 24
	}

	return time.Duration(periodHours) * time.Hour
}

func KubeCreateServingCertificate(ctx context.Context, r *ReconcileBase, hawtio commonapi.HawtioGeneral) (*corev1.Secret, time.Duration, error) {
	// This secret name should be the same as used in deployment.go
	metadataInfo := hawtio.GetMetadataInfo()
	servingSecretName := metadataInfo.Name + "-tls-serving"
	rotationWindow := getRotationBufferWindow(hawtio)

	// Check whether serving certificate secret exists
	servingCertSecret, err := r.CoreClient.Secrets(metadataInfo.Namespace).Get(ctx, servingSecretName, metav1.GetOptions{})
	if err == nil {
		// Found the secret

		// Check the secret's labels
		labels := servingCertSecret.GetLabels()
		if labels == nil || labels[resources.LabelAppKey] != resources.LabelAppValue {
			// This a legacy certificate so adopt it
			// Note: adoptLegacyResource returns the Sentinel Error (ErrLegacyResourceAdopted)
			// on success.
			adoptErr := r.AdoptLegacyResource(ctx, servingCertSecret)
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
		expiryIn := checkCertificateExpiry(rotationWindow, servingCertSecret, r.Logger)
		if expiryIn == 0 {
			// certificate is invalid or close to expiring
			// create a new one and update the secret
			newSecret, err := newSelfCertificateSecret(ctx, r, hawtio, servingCertSecret.Name, servingCertSecret.Namespace)
			if err != nil {
				return nil, 0, err
			}

			// Initialize the Data map on the existing secret if it's somehow nil
			if servingCertSecret.Data == nil {
				servingCertSecret.Data = make(map[string][]byte)
			}

			// Transplant the fresh crypto material into the existing object
			servingCertSecret.Data[corev1.TLSCertKey] = newSecret.Data[corev1.TLSCertKey]
			servingCertSecret.Data[corev1.TLSPrivateKeyKey] = newSecret.Data[corev1.TLSPrivateKeyKey]

			// Commit the update
			if err := r.Client.Update(ctx, servingCertSecret); err != nil {
				return nil, 0, err
			}

			// reset expiryIn to maximum as new certificate
			expiryIn = checkCertificateExpiry(rotationWindow, servingCertSecret, r.Logger)
		}

		return servingCertSecret, expiryIn, nil
	}

	if kerrors.IsNotFound(err) {
		conKLog.Info("Serving certificate secret not found, creating a new one", "secret", servingSecretName)

		servingCertSecret, err := newSelfCertificateSecret(ctx, r, hawtio, servingSecretName, metadataInfo.Namespace)
		if err != nil {
			return nil, 0, err
		}

		err = controllerutil.SetControllerReference(hawtio, servingCertSecret, r.Scheme)
		if err != nil {
			return nil, 0, err
		}
		_, err = r.CoreClient.Secrets(metadataInfo.Namespace).Create(ctx, servingCertSecret, metav1.CreateOptions{})
		if err != nil {
			return nil, 0, errs.Wrap(err, "Creating the serving certificate secret failed")
		}

		conKLog.Info("Serving certificate created successfully", "secret", servingSecretName)
		// New Secret so maximum expiry period with buffer
		expiryIn := checkCertificateExpiry(rotationWindow, servingCertSecret, r.Logger)
		return servingCertSecret, expiryIn, nil
	}

	// error was something but not NotFound
	return nil, 0, err
}
