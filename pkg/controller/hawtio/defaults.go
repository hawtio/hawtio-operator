package hawtio

import (
	"context"
	"fmt"
	"time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"

	"github.com/hawtio/hawtio-operator/pkg/util"
)

// Cap the maximum requeue time to 24 hours
// Maximum time to sleep before a requeue should take place
var maxRequeueTime = 24 * time.Hour

func (r *ReconcileHawtio) verifyHawtioSpecType(ctx context.Context, hawtio *hawtiov2.Hawtio) (bool, error) {
	if len(hawtio.Spec.Type) == 0 {
		r.logger.V(util.DebugLogLevel).Info("Hawtio.Spec.Type not specified. Defaulting to Cluster")

		previous := hawtio.DeepCopy()
		hawtio.Spec.Type = hawtiov2.ClusterHawtioDeploymentType

		// Use patch rather than update to ensure only the
		// explicit changes are merged in rather than potentially
		// overwriting with a stale hawtio CR
		err := r.client.Patch(ctx, hawtio, client.MergeFrom(previous))
		if err != nil {
			return false, fmt.Errorf("failed to update type: %v", err)
		}

		return true, nil // CR was updated with no error
	}

	if hawtio.Spec.Type != hawtiov2.NamespaceHawtioDeploymentType && (hawtio.Spec.Type != hawtiov2.ClusterHawtioDeploymentType) {
		r.logger.V(util.DebugLogLevel).Info("Hawtio.Spec.Type neither Cluster or Namespace")

		err := r.setHawtioPhase(ctx, hawtio, hawtiov2.HawtioPhaseFailed)
		if err != nil {
			return false, err
		}

		return true, fmt.Errorf("unsupported type: %s", hawtio.Spec.Type) // CR was updated and report unsupported type error
	}

	return false, nil // CR was not update and spec type checked out
}

func (r *ReconcileHawtio) findConsoleURL(ctx context.Context) (string, error) {
	if ! r.apiSpec.IsOpenShift4 {
		return "", nil
	}

	//
	// === Find the OCP Console Public URL ===
	//
	cm, err := r.coreClient.ConfigMaps("openshift-config-managed").Get(ctx, "console-public", metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) && !kerrors.IsForbidden(err) {
			r.logger.Error(err, "Error getting OpenShift managed configuration")
			return "", err
		}
	}

	return cm.Data["consoleURL"], nil
}

//
// resolveProxyClientCertificate determines existence and validity
// of the proxy certificate.
// Returns (certificate secret, time before rotation required, error)
//
func (r *ReconcileHawtio) resolveProxyClientCertificate(ctx context.Context, hawtio *hawtiov2.Hawtio) (*corev1.Secret, time.Duration, error) {
	if ! r.apiSpec.IsOpenShift4 {
		return nil, 0, nil // not required on Kubernetes
	}

	r.logger.V(util.DebugLogLevel).Info("Resolving OpenShift proxying certificate")

	//
	// Create -proxying certificate - only applicable for OCP
	//
	clientCertSecret, expiryIn, err := osCreateClientCertificate(ctx, r, hawtio)
	if err != nil {
		if err == ErrLegacyResourceAdopted {
			r.logger.Error(err, "OpenShift proxying certificate exists but need to adopt")
		} else {
			r.logger.Error(err, "Failed to create OpenShift proxying certificate")
		}
		return nil, 0, err
	}

	// Can be nil in tests
	if clientCertSecret != nil {
		// Set the owner reference for garbage collection.
		err = controllerutil.SetControllerReference(hawtio, clientCertSecret, r.scheme)
		if err != nil {
			return nil, 0, err
		}
	}

	return clientCertSecret, expiryIn, nil
}

func (r *ReconcileHawtio) resolveServingClientCertificate(ctx context.Context, hawtio *hawtiov2.Hawtio) (*corev1.Secret, time.Duration, error) {
	if r.apiSpec.IsOpenShift4 {
		// -serving certificate is automatically created on OCP
		return nil, 0, nil // not required on OCP
	}

	r.logger.V(util.DebugLogLevel).Info("Resolving Kubernetes serving certificate")

	// Create -serving certificate
	servingCertSecret, expiryIn, err := kubeCreateServingCertificate(ctx, r, hawtio)
	if err != nil {
		if err == ErrLegacyResourceAdopted {
			r.logger.Error(err, "Kube serving certificate exists but need to adopt")
		} else {
			r.logger.Error(err, "Failed to create serving certificate")
		}
		return nil, 0, err
	}

	// Can be nil in tests
	if servingCertSecret != nil {
		// Set the owner reference for garbage collection.
		err = controllerutil.SetControllerReference(hawtio, servingCertSecret, r.scheme)
		if err != nil {
			return nil, 0, err
		}
	}

	return servingCertSecret, expiryIn, nil
}

func (r *ReconcileHawtio) resolveRouteCertificate(ctx context.Context, hawtio *hawtiov2.Hawtio) (*corev1.Secret, error) {
	secretName := hawtio.Spec.Route.CertSecret.Name
	if secretName == "" {
		return nil, nil // no secret specified
	}

	r.logger.V(util.DebugLogLevel).Info("Assigning Hawtio.Spec.Route certificate secret to deployment")

	tlsRouteSecret, err := r.coreClient.Secrets(hawtio.Namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// Structural Validation
	if len(tlsRouteSecret.Data[corev1.TLSPrivateKeyKey]) == 0 || len(tlsRouteSecret.Data[corev1.TLSCertKey]) == 0 {
		err := fmt.Errorf("custom route secret %s is missing required keys: tls.crt and/or tls.key", secretName)
		r.logger.Error(err, "Invalid custom certificate secret")
		return nil, err
	}

	// User mounted certificate so should NOT be adopted as a legacy resource or operator owned
	return tlsRouteSecret, nil
}

func (r *ReconcileHawtio) resolveRouteCACertificate(ctx context.Context, hawtio *hawtiov2.Hawtio) (*corev1.Secret, error) {
	caCertSecretName := hawtio.Spec.Route.CaCert.Name
	if caCertSecretName == "" {
		return nil, nil // no secret specified
	}

	r.logger.V(util.DebugLogLevel).Info("Assigning Hawtio.Spec.Route CA certificate secret to deployment")

	caRouteSecret, err := r.coreClient.Secrets(hawtio.Namespace).Get(ctx, caCertSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// Structural Validation
	if len(caRouteSecret.Data[corev1.TLSPrivateKeyKey]) == 0 || len(caRouteSecret.Data[corev1.TLSCertKey]) == 0 {
		err := fmt.Errorf("custom route secret %s is missing required keys: tls.crt and/or tls.key", caCertSecretName)
		r.logger.Error(err, "Invalid custom certificate secret")
		return nil, err
	}

	// User mounted certificate so should NOT be adopted as a legacy resource or operator owned
	return caRouteSecret, nil
}

func (r *ReconcileHawtio) initDeploymentConfiguration(ctx context.Context, hawtio *hawtiov2.Hawtio) (DeploymentConfiguration, error) {
	deploymentConfiguration := DeploymentConfiguration{}

	//
	// Find the OpenShift Console URL if appropriate
	//
	r.logger.V(util.DebugLogLevel).Info("Setting console URL on deployment")
	url, err := r.findConsoleURL(ctx)
	if err != nil {
		return deploymentConfiguration, err
	}
	deploymentConfiguration.openShiftConsoleURL = url

	//
	// Log if deprecated clientCertCheckSchedule still used in CR
	//
	if hawtio.Spec.Auth.ClientCertCheckSchedule != "" {
		r.logger.Info("Notice: clientCertCheckSchedule is deprecated in v2+ and is being ignored. " +
			"Certificate rotation is now handled natively by the controller.")
	}

	//
	// Create, find or update a proxy client certificate if appropriate
	//
	proxySecret, expiryIn, err := r.resolveProxyClientCertificate(ctx, hawtio)
	if err != nil {
		return deploymentConfiguration, err
	}
	deploymentConfiguration.clientCertSecret = proxySecret
	// Sleep for a maximum of maxRequeueTime
	deploymentConfiguration.requeueAfter = min(expiryIn, maxRequeueTime)

	//
	// Create, find or update a serving client certificate if appropriate
	//
	servingSecret, expiryIn, err := r.resolveServingClientCertificate(ctx, hawtio)
	if err != nil {
		return deploymentConfiguration, err
	}
	deploymentConfiguration.servingCertSecret = servingSecret

	// If the serving cert has a valid expiration timer...
	if expiryIn > 0 {
		// Adopt it IF no timer yet, OR if it's shorter than the current timer
		if deploymentConfiguration.requeueAfter == 0 || expiryIn < deploymentConfiguration.requeueAfter {
			// Sleep for a maximum of maxRequeueTime
			deploymentConfiguration.requeueAfter = min(expiryIn, maxRequeueTime)
		}
	}

	//
	// Custom Route certificate defined in Hawtio CR
	//
	tlsRouteSecret, err := r.resolveRouteCertificate(ctx, hawtio)
	if err != nil {
		return deploymentConfiguration, err
	}
	deploymentConfiguration.tlsRouteSecret = tlsRouteSecret

	//
	// Custom Route CA certificate defined in Hawtio CR
	//
	caCertRouteSecret, err := r.resolveRouteCACertificate(ctx, hawtio)
	if err != nil {
		return deploymentConfiguration, err
	}
	deploymentConfiguration.caCertRouteSecret = caCertRouteSecret

	return deploymentConfiguration, nil
}

// hydrateDefaults performs a server-side Dry-Run Create to populate the
// 'blueprint' object with all the default values (Spec, Status, etc.) that
// the specific cluster applies.
// It accepts an optional ensureSpecIntegrity closure to validate that
// critical fields remain present and if not restore them from the blueprint
// object.
// It returns the "hydrated" object.
func hydrateDefaults[T client.Object](ctx context.Context, c client.Client, blueprint T, ensureSpecIntegrity func(source T, hydrated T)) (T, error) {
	// DeepCopy to avoid mutating the input object in case the caller reuses it
	// Cast the result back to T (which works because T is a pointer to a struct)
	obj := blueprint.DeepCopyObject().(T)

	// OpenShift OAuthClient validation rejects it.
	obj.SetGenerateName("")

	// Generate a unique, valid name.
	// Ends with a number, satisfying the [a-z0-9] regex constraint.
	// Use a very short name (t + 7 digits) to prevent DNS length errors in Routes.
	// Example: t4829102
	obj.SetName(fmt.Sprintf("t%d", time.Now().UnixNano()%10000000))

	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetSelfLink("")
	obj.SetCreationTimestamp(metav1.Time{})
	// Clear OwnerReferences just in case, though DryRun usually ignores them
	obj.SetOwnerReferences(nil)

	// Perform Dry-Run Create
	// The API server will populate defaults and return the object
	if err := c.Create(ctx, obj, client.DryRunAll); err != nil {
		// We return the 'zero' value of T and the error
		var zero T
		return zero, fmt.Errorf("failed to hydrate defaults via dry-run: %w", err)
	}

	// Validate with the ensureSpecIntegrity function if one was provided
	// to patch any dropped fields
	if ensureSpecIntegrity != nil {
		ensureSpecIntegrity(blueprint, obj)
	}

	return obj, nil
}
