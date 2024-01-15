package hawtio

import (
	"context"
	"time"

	hawtiov1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1"
	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var conKLog = logf.Log.WithName("controller_hawtio_kubernetes")

func kubeCreateServingCertificate(ctx context.Context, r *ReconcileHawtio, hawtio *hawtiov1.Hawtio, name string, namespace string) (*corev1.Secret, error) {
	// This secret name should be the same as used in deployment.go
	servingSecretName := hawtio.Name + "-tls-serving"

	// Check whether serving certificate secret exists
	servingCertSecret, err := r.coreClient.Secrets(namespace).Get(ctx, servingSecretName, metav1.GetOptions{})
	if err == nil {
		return servingCertSecret, nil
	}

	if kerrors.IsNotFound(err) {
		conKLog.Info("Serving certificate secret not found, creating a new one", "secret", servingSecretName)

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
		servingCertSecret, err := generateSelfSignedCertSecret(servingSecretName, namespace, commonName, expirationDate)
		if err != nil {
			return nil, errs.Wrap(err, "Generating the serving certificate failed")
		}
		err = controllerutil.SetControllerReference(hawtio, servingCertSecret, r.scheme)
		if err != nil {
			return nil, err
		}
		_, err = r.coreClient.Secrets(namespace).Create(ctx, servingCertSecret, metav1.CreateOptions{})
		if err != nil {
			return nil, errs.Wrap(err, "Creating the serving certificate secret failed")
		}

		conKLog.Info("Serving certificate created successfully", "secret", servingSecretName)
		return servingCertSecret, nil
	}

	return nil, err
}
