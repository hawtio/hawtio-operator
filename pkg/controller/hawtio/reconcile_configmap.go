package hawtio

import (
	"context"
	"fmt"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"

	"github.com/hawtio/hawtio-operator/pkg/resources"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

func (r *ReconcileHawtio) reconcileConfigMap(ctx context.Context, hawtio *hawtiov2.Hawtio) (*corev1.ConfigMap, controllerutil.OperationResult, error) {
	configMap := resources.NewDefaultConfigMap(hawtio)

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, configMap, func() error {
		// A read-only copy of the cluster state for diff logging
		liveSnapshot := configMap.DeepCopy()

		// Set the owner reference for garbage collection.
		if err := controllerutil.SetControllerReference(hawtio, configMap, r.scheme); err != nil {
			return err
		}

		// Get the target state for the ConfigMap
		reqLogger := hawtioLogger.WithName(fmt.Sprintf("%s-reconcileConfigMap", hawtio.Name))
		crConfigMap, err := resources.NewConfigMap(hawtio, r.apiSpec, reqLogger)
		if err != nil {
			reqLogger.Error(err, "Error reconciling ConfigMap")
			return err
		}

		// Merge Metadata (Crucial for stability)
		// We merge so we don't wipe out system labels/annotations
		configMap.Labels = util.MergeMap(configMap.Labels, crConfigMap.Labels)
		configMap.Annotations = util.MergeMap(configMap.Annotations, crConfigMap.Annotations)

		// Mutate the object's Data field to match the source state.
		// No hydration needed because K8s doesn't default this field.
		configMap.Data = crConfigMap.Data
		configMap.BinaryData = crConfigMap.BinaryData

		// Report any known differences to the log (only if in debug log level)
		util.ReportDiff("ConfigMap", liveSnapshot, configMap)

		return nil
	})

	// There is a resource but the default client cache cannot see it
	// since it is legacy and not labelled. The create failed so it
	// should be adopted and reconciled.
	if err != nil && kerrors.IsAlreadyExists(err) {
		adoptionErr := r.adoptLegacyResource(ctx, resources.NewDefaultConfigMap(hawtio))
		if adoptionErr != nil {
			// If adoption failed (e.g., API error) or an adoption was called
			// return any of these errors
			return nil, controllerutil.OperationResultNone, adoptionErr
		}
	}

	// Any other error
	if err != nil {
		return nil, opResult, err
	}

	util.ReportResourceChange("ConfigMap", configMap, opResult)
	return configMap, opResult, nil
}
