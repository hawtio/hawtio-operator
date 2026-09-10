package hawtio

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/hawtio/hawtio-operator/pkg/util"
)

func (r *ReconcileHawtio) getOperatorOwner(ctx context.Context) (metav1.Object, error) {
	r.logger.V(util.DebugLogLevel).Info("Getting operator owner ...")

	// Get the Operator Pod using r.operatorPod
	pod := &corev1.Pod{}
	err := r.apiReader.Get(ctx, r.operatorPod, pod)
	if err != nil {
		return nil, err
	}

	r.logger.V(util.DebugLogLevel).Info("Found Operator Pod", "Pod", pod.Name, "Namespace", r.operatorPod.Namespace)

	// Get the ReplicaSet Name from the Pod's OwnerReferences
	podOwner := metav1.GetControllerOf(pod)
	if podOwner == nil || podOwner.Kind != "ReplicaSet" {
		// Fallback to owning Pod if not managed by a ReplicaSet (e.g., local testing)
		return pod, nil
	}

	r.logger.V(util.DebugLogLevel).Info("Operator Pod Owner", "Owner", podOwner.Name, "Namespace", r.operatorPod.Namespace)

	// Fetch the ReplicaSet
	replicaSet := &appsv1.ReplicaSet{}
	err = r.apiReader.Get(ctx, client.ObjectKey{Namespace: r.operatorPod.Namespace, Name: podOwner.Name}, replicaSet)
	if err != nil {
		return nil, err
	}

	r.logger.V(util.DebugLogLevel).Info("Found Operator ReplicaSet", "ReplicaSet", replicaSet.Name, "Namespace", r.operatorPod.Namespace)

	// Get the Deployment Name from the ReplicaSet's OwnerReferences
	rsOwner := metav1.GetControllerOf(replicaSet)
	if rsOwner == nil || rsOwner.Kind != "Deployment" {
		// Fallback to ReplicaSet if no Deployment owner exists
		return replicaSet, nil
	}

	r.logger.V(util.DebugLogLevel).Info("Operator ReplicaSet Owner", "Owner", rsOwner.Name, "Namespace", r.operatorPod.Namespace)

	// Fetch the Deployment struct
	deployment := &appsv1.Deployment{}
	err = r.apiReader.Get(ctx, client.ObjectKey{Namespace: r.operatorPod.Namespace, Name: rsOwner.Name}, deployment)
	if err != nil {
		return nil, err
	}

	r.logger.V(util.DebugLogLevel).Info("Found Operator Deployment", "Deployment", deployment.Name, "Namespace", r.operatorPod.Namespace)

	return deployment, nil
}

func (r *ReconcileHawtio) getHawtioOwner(ctx context.Context, hawtio *hawtiov2.Hawtio) (string, error) {
	r.logger.V(util.DebugLogLevel).Info("Retrieving Hawtio CR Owner")

	user := hawtio.Annotations["hawtio.io/last-modified-by"]
	if len(user) == 0 {
		return "", fmt.Errorf("The owner of the Hawtio CR %s cannot be determined", hawtio.Name)
	}

	r.logger.V(util.DebugLogLevel).Info("Hawtio CR auditing", "modified-by", user)
	return user, nil
}

func (r *ReconcileHawtio) validateResourceAccess(ctx context.Context, hawtio *hawtiov2.Hawtio, resourceAttr *authorizationv1.ResourceAttributes) error {
	r.logger.V(util.DebugLogLevel).Info("Validating Resource Access", "resource", resourceAttr)

	user, err := r.getHawtioOwner(ctx, hawtio)
	if err != nil {
		return err
	}

	sar := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			ResourceAttributes: resourceAttr,
			User:               user,
		},
	}

	result, err := r.apiClient.AuthorizationV1().SubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("authorization check failed: %w", err)
	}

	if !result.Status.Allowed {
		return fmt.Errorf("user %s not authorized for custom hosts: %s", user, result.Status.Reason)
	}

	r.logger.V(util.DebugLogLevel).Info("User access to resource allowed", "user", user, "resource", resourceAttr)

	return nil
}
