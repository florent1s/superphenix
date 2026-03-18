package cluster

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

func (r *Reconciler) setCondition(conditions *[]metav1.Condition, newCondition metav1.Condition) {
	for i, c := range *conditions {
		if c.Type == newCondition.Type {
			if c.Status == newCondition.Status && c.Reason == newCondition.Reason && c.Message == newCondition.Message {
				return
			}
			(*conditions)[i] = newCondition
			return
		}
	}
	*conditions = append(*conditions, newCondition)
}

func (r *Reconciler) updateStatus(ctx context.Context, cluster *operatorv1alpha1.Cluster, condType string, status metav1.ConditionStatus, reason, message string) {
	log := logf.FromContext(ctx)
	patch := client.MergeFrom(cluster.DeepCopy())
	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: cluster.Generation,
	}

	// Ensure opposite conditions are updated too
	if condType == operatorv1alpha1.ConditionTypeConnected && status == metav1.ConditionTrue {
		r.setCondition(&cluster.Status.Conditions, condition)
		r.setCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               operatorv1alpha1.ConditionTypeUnreachable,
			Status:             metav1.ConditionFalse,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: cluster.Generation,
		})
	} else if condType == operatorv1alpha1.ConditionTypeUnreachable && status == metav1.ConditionTrue {
		r.setCondition(&cluster.Status.Conditions, condition)
		r.setCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               operatorv1alpha1.ConditionTypeConnected,
			Status:             metav1.ConditionFalse,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: cluster.Generation,
		})
	} else {
		r.setCondition(&cluster.Status.Conditions, condition)
	}

	cluster.Status.ObservedGeneration = cluster.Generation

	if err := r.Status().Patch(ctx, cluster, patch); err != nil {
		log.Error(err, "Failed to patch Cluster status")
	}
}
