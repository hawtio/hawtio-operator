package common

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/hawtio/hawtio-operator/pkg/util"
)

func (r *ReconcileBase) GetOperatorOwner(ctx context.Context) (metav1.Object, error) {
	r.Logger.V(util.DebugLogLevel).Info("Getting operator owner ...")

	// Get the Operator Pod using r.OperatorPod
	pod := &corev1.Pod{}
	err := r.ApiReader.Get(ctx, r.OperatorPod, pod)
	if err != nil {
		return nil, err
	}

	r.Logger.V(util.DebugLogLevel).Info("Found Operator Pod", "Pod", pod.Name, "Namespace", r.OperatorPod.Namespace)

	// Get the ReplicaSet Name from the Pod's OwnerReferences
	podOwner := metav1.GetControllerOf(pod)
	if podOwner == nil || podOwner.Kind != "ReplicaSet" {
		// Fallback to owning Pod if not managed by a ReplicaSet (e.g., local testing)
		return pod, nil
	}

	r.Logger.V(util.DebugLogLevel).Info("Operator Pod Owner", "Owner", podOwner.Name, "Namespace", r.OperatorPod.Namespace)

	// Fetch the ReplicaSet
	replicaSet := &appsv1.ReplicaSet{}
	err = r.ApiReader.Get(ctx, client.ObjectKey{Namespace: r.OperatorPod.Namespace, Name: podOwner.Name}, replicaSet)
	if err != nil {
		return nil, err
	}

	r.Logger.V(util.DebugLogLevel).Info("Found Operator ReplicaSet", "ReplicaSet", replicaSet.Name, "Namespace", r.OperatorPod.Namespace)

	// Get the Deployment Name from the ReplicaSet's OwnerReferences
	rsOwner := metav1.GetControllerOf(replicaSet)
	if rsOwner == nil || rsOwner.Kind != "Deployment" {
		// Fallback to ReplicaSet if no Deployment owner exists
		return replicaSet, nil
	}

	r.Logger.V(util.DebugLogLevel).Info("Operator ReplicaSet Owner", "Owner", rsOwner.Name, "Namespace", r.OperatorPod.Namespace)

	// Fetch the Deployment struct
	deployment := &appsv1.Deployment{}
	err = r.ApiReader.Get(ctx, client.ObjectKey{Namespace: r.OperatorPod.Namespace, Name: rsOwner.Name}, deployment)
	if err != nil {
		return nil, err
	}

	r.Logger.V(util.DebugLogLevel).Info("Found Operator Deployment", "Deployment", deployment.Name, "Namespace", r.OperatorPod.Namespace)

	return deployment, nil
}
