package hawtio

import (
  "context"
  "fmt"
  "time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

  corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

  "sigs.k8s.io/controller-runtime/pkg/client"

  hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"

	"github.com/hawtio/hawtio-operator/pkg/util"
)

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

func (r *ReconcileHawtio) initDeploymentConfiguration(ctx context.Context, hawtio *hawtiov2.Hawtio, namespacedName client.ObjectKey) (DeploymentConfiguration, error) {
	deploymentConfiguration := DeploymentConfiguration{}

	if r.apiSpec.IsOpenShift4 {
		//
		// === Find the OCP Console Public URL ===
		//
		r.logger.V(util.DebugLogLevel).Info("Setting console URL on deployment")

		cm, err := r.coreClient.ConfigMaps("openshift-config-managed").Get(ctx, "console-public", metav1.GetOptions{})
		if err != nil {
			if !kerrors.IsNotFound(err) && !kerrors.IsForbidden(err) {
				r.logger.Error(err, "Error getting OpenShift managed configuration")
				return deploymentConfiguration, err
			}
		} else {
			deploymentConfiguration.openShiftConsoleURL = cm.Data["consoleURL"]
		}

		r.logger.V(util.DebugLogLevel).Info("Creating OpenShift proxying certificate")

		//
		// === Create -proxying certificate - only applicable for OCP ===
		// === -serving certificate is automatically created on OCP ===
		//
		clientCertSecret, err := osCreateClientCertificate(ctx, r, hawtio, namespacedName.Name, namespacedName.Namespace)
		if err != nil {
			if err == ErrLegacyResourceAdopted {
				r.logger.Error(err, "OpenShift proxying certificate exists but need to adopt")
			} else {
				r.logger.Error(err, "Failed to create OpenShift proxying certificate")
			}
			return deploymentConfiguration, err
		}
		deploymentConfiguration.clientCertSecret = clientCertSecret
	} else {
		//
		// === Create the Kubernetes serving certificate ===
		//
		r.logger.V(util.DebugLogLevel).Info("Creating Kubernetes serving certificate")

		// Create -serving certificate
		servingCertSecret, err := kubeCreateServingCertificate(ctx, r, hawtio, namespacedName.Name, namespacedName.Namespace)
		if err != nil {
			if err == ErrLegacyResourceAdopted {
				r.logger.Error(err, "Kube serving certificate exists but need to adopt")
			} else {
				r.logger.Error(err, "Failed to create serving certificate")
			}
			return deploymentConfiguration, err
		}
		deploymentConfiguration.servingCertSecret = servingCertSecret
	}

	//
	// === Custom Route certificate defined in Hawtio CR ===
	//
	if secretName := hawtio.Spec.Route.CertSecret.Name; secretName != "" {
		r.logger.V(util.DebugLogLevel).Info("Assigning Hawtio.Spec.Route certificate secret to deployment")
		deploymentConfiguration.tlsRouteSecret = &corev1.Secret{}
		err := r.client.Get(ctx, client.ObjectKey{Namespace: namespacedName.Namespace, Name: secretName}, deploymentConfiguration.tlsRouteSecret)
		if err != nil {
			return deploymentConfiguration, err
		}
	}

	//
	// === Custom Route CA certificate defined in Hawtio CR ===
	//
	if caCertSecretName := hawtio.Spec.Route.CaCert.Name; caCertSecretName != "" {
		r.logger.V(util.DebugLogLevel).Info("Assigning Hawtio.Spec.Route CA secret to deploment")
		deploymentConfiguration.caCertRouteSecret = &corev1.Secret{}
		err := r.client.Get(ctx, client.ObjectKey{Namespace: namespacedName.Namespace, Name: caCertSecretName}, deploymentConfiguration.caCertRouteSecret)
		if err != nil {
			return deploymentConfiguration, err
		}
	}

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
