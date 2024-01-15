package hawtio

import (
	"context"
	"time"

	hawtiov1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1"
	errs "github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var conOsLog = logf.Log.WithName("controller_hawtio_openshift")

func osCreateClientCertificate(ctx context.Context, r *ReconcileHawtio, hawtio *hawtiov1.Hawtio, name string, namespace string) (*corev1.Secret, error) {
	// This secret name should be the same as used in deployment.go
	clientSecretName := hawtio.Name + "-tls-proxying"

	cronJob := &batchv1.CronJob{}
	cronJobName := name + "-certificate-expiry-check"
	cronJobErr := r.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: cronJobName}, cronJob)

	// Check whether client certificate secret exists
	clientCertSecret, err := r.coreClient.Secrets(namespace).Get(ctx, clientSecretName, metav1.GetOptions{})
	if err == nil {
		return clientCertSecret, nil
	}

	if kerrors.IsNotFound(err) {
		conOsLog.Info("Client certificate secret not found, creating a new one", "secret", clientSecretName)

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
		clientCertSecret, err := generateCASignedCertSecret(clientSecretName, namespace, caSecret, commonName, expirationDate)
		if err != nil {
			return nil, errs.Wrap(err, "Generating the client certificate failed")
		}
		err = controllerutil.SetControllerReference(hawtio, clientCertSecret, r.scheme)
		if err != nil {
			return nil, err
		}
		_, err = r.coreClient.Secrets(namespace).Create(ctx, clientCertSecret, metav1.CreateOptions{})
		conOsLog.Info("Client certificate created successfully", "secret", clientSecretName)
		if err != nil {
			return nil, errs.Wrap(err, "Creating the client certificate secret failed")
		}

		// check if certificate rotation is enabled
		if hawtio.Spec.Auth.ClientCertCheckSchedule != "" {
			// generate auto-renewal cron job for the secret if it already hasn't been generated.
			if cronJobErr != nil && kerrors.IsNotFound(cronJobErr) {
				pod, err := getOperatorPod(ctx, r.client, namespace)
				if err != nil {
					return nil, err
				}

				//create cronJob to validate the Cert
				cronJob = createCertValidationCronJob(cronJobName, namespace,
					hawtio.Spec.Auth.ClientCertCheckSchedule, pod.Spec.ServiceAccountName, pod.Spec.Containers[0],
					hawtio.Spec.Auth.ClientCertExpirationPeriod)

				err = controllerutil.SetControllerReference(hawtio, cronJob, r.scheme)
				if err != nil {
					return nil, err
				}
				err = r.client.Create(ctx, cronJob)

				if err != nil {
					return nil, err
				}
			}
		}

		return clientCertSecret, nil
	}

	return nil, err
}
