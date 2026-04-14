package hawtio

import (
	"context"
	"fmt"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	batchv1 "k8s.io/api/batch/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"

	"github.com/hawtio/hawtio-operator/pkg/resources"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

func (r *ReconcileHawtio) reconcileCronJob(ctx context.Context, hawtio *hawtiov2.Hawtio, crNamespacedName client.ObjectKey, opNamespacedName client.ObjectKey, deploymentConfig DeploymentConfiguration) (controllerutil.OperationResult, error) {
	if deploymentConfig.clientCertSecret == nil {
		// No certificate so cronjob not necessary
		return controllerutil.OperationResultNone, nil
	}

	// Determine if the CronJob should exist.
	shouldExist := hawtio.Spec.Auth.ClientCertCheckSchedule != ""
	cronJobName := hawtio.Name + "-certificate-expiry-check"

	if shouldExist {
		// The CronJob SHOULD exist. ---
		r.logger.Info("Ensuring CronJob exists and is up to date", "CronJob.Name", cronJobName, "CronJob.Namespace", crNamespacedName.Namespace)
		targetCronJob := resources.NewDefaultCronJob(hawtio)

		opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, targetCronJob, func() error {
			// A read-only copy of the cluster state for diff logging
			liveSnapshot := targetCronJob.DeepCopy()

			// Set the owner reference for garbage collection.
			if err := controllerutil.SetControllerReference(hawtio, targetCronJob, r.scheme); err != nil {
				return err
			}

			// Need to get the operator from the
			// operator namespace and not the CR namespace
			pod, err := r.getOperatorPod(ctx, opNamespacedName)
			if err != nil {
				return err
			}

			blueprint, err := resources.NewCronJob(hawtio, pod, crNamespacedName.Namespace)
			if err != nil {
				return fmt.Errorf("failed to build source cronjob: %w", err)
			}

			serverBlueprint, err := hydrateDefaults(ctx, r.client, blueprint, func(source, hydrated *batchv1.CronJob) {
				// If hydration stripped required fields, patch them directly back from source
				if hydrated.Spec.Schedule == "" {
					r.logger.Info("CronJob hydration dropped the Schedule. Restoring from source spec.",
						"CronJob.Name", source.Name)
					hydrated.Spec.Schedule = source.Spec.Schedule
				}
				if len(hydrated.Spec.JobTemplate.Spec.Template.Spec.Containers) == 0 {
					r.logger.Info("CronJob hydration dropped Containers. Restoring from source spec.",
						"CronJob.Name", source.Name)
					hydrated.Spec.JobTemplate.Spec.Template.Spec.Containers = source.Spec.JobTemplate.Spec.Template.Spec.Containers
				}
			})
			if err != nil {
				return err
			}

			targetCronJob.Labels = util.MergeMap(targetCronJob.Labels, blueprint.Labels)
			targetCronJob.Annotations = util.MergeMap(targetCronJob.Annotations, blueprint.Annotations)
			targetCronJob.Spec = serverBlueprint.Spec

			// Report any known differences to the log (only if in debug log level)
			util.ReportDiff("CronJob", liveSnapshot, targetCronJob)

			return nil
		})

		// There is a resource but the default client cache cannot see it
		// since it is legacy and not labelled. The create failed so it
		// should be adopted and reconciled.
		if err != nil && kerrors.IsAlreadyExists(err) {
			adoptionErr := r.adoptLegacyResource(ctx, resources.NewDefaultCronJob(hawtio))
			if adoptionErr != nil {
				// If adoption failed (e.g., API error) or an adoption was called
				// return any of these errors
				return controllerutil.OperationResultNone, adoptionErr
			}
		}

		if err != nil {
			return opResult, err
		}

		util.ReportResourceChange("CronJob", targetCronJob, opResult)
		return opResult, nil
	} else {
		// The CronJob SHOULD NOT exist. ---
		// We must ensure it is deleted if it's found.
		r.logger.V(util.DebugLogLevel).Info("Ensuring CronJob does not exist", "CronJob.Name", cronJobName)

		staleCronJob := &batchv1.CronJob{}
		err := r.client.Get(ctx, types.NamespacedName{Name: cronJobName, Namespace: crNamespacedName.Namespace}, staleCronJob)

		if err != nil {
			if kerrors.IsNotFound(err) {
				// It doesn't exist, which is what we want. Success.
				return controllerutil.OperationResultNone, nil
			}
			return controllerutil.OperationResultNone, err // A real error occurred.
		}

		// If we found it, it's a stale resource that needs to be deleted.
		r.logger.Info("Deleting stale CronJob", "CronJob.Name", cronJobName)
		if err := r.client.Delete(ctx, staleCronJob); err != nil {
			return controllerutil.OperationResultNone, err
		}

		return controllerutil.OperationResultUpdated, nil // Signifies a change (deletion) was made.
	}
}
