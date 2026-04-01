package cluster

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
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
	r.updateStatusWithPhase(ctx, cluster, condType, status, reason, message, "")
}

func (r *Reconciler) updateStatusWithPhase(ctx context.Context, cluster *operatorv1alpha1.Cluster, condType string, status metav1.ConditionStatus, reason, message string, phase string) {
	log := logf.FromContext(ctx)

	var existingCondition *metav1.Condition
	for i := range cluster.Status.Conditions {
		if cluster.Status.Conditions[i].Type == condType {
			existingCondition = &cluster.Status.Conditions[i]
			break
		}
	}

	// Check if any change is needed
	changed := false
	if phase != "" && cluster.Status.Phase != phase {
		changed = true
	}
	if cluster.Status.ObservedGeneration != cluster.Generation {
		changed = true
	}
	if existingCondition == nil {
		changed = true
	} else if existingCondition.Status != status || existingCondition.Reason != reason || existingCondition.Message != message {
		changed = true
	}

	if !changed {
		return
	}

	patch := client.MergeFrom(cluster.DeepCopy())

	if phase != "" {
		cluster.Status.Phase = phase
	}

	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cluster.Generation,
	}

	if existingCondition != nil && existingCondition.Status == status && existingCondition.Reason == reason && existingCondition.Message == message {
		condition.LastTransitionTime = existingCondition.LastTransitionTime
	} else {
		condition.LastTransitionTime = metav1.Now()
	}

	r.setCondition(&cluster.Status.Conditions, condition)
	cluster.Status.ObservedGeneration = cluster.Generation

	if err := r.Status().Patch(ctx, cluster, patch); err != nil {
		log.Error(err, "Failed to patch Cluster status")
	}
}

// updateKubernetesVersion updates the Kubernetes version in the cluster status if it changed.
func (r *Reconciler) updateKubernetesVersion(ctx context.Context, cluster *operatorv1alpha1.Cluster, k8sVersion string) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if cluster.Status.KubernetesVersion == k8sVersion {
		return ctrl.Result{}, nil
	}

	log.Info("Updating Kubernetes version", "oldVersion", cluster.Status.KubernetesVersion, "newVersion", k8sVersion)
	patch := client.MergeFrom(cluster.DeepCopy())
	cluster.Status.KubernetesVersion = k8sVersion

	if err := r.Status().Patch(ctx, cluster, patch); err != nil {
		log.Error(err, "Failed to patch cluster status with kubernetes version")
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}
	return ctrl.Result{}, nil
}

// updateClusterVersion updates the current version and phase in the cluster status if they changed.
func (r *Reconciler) updateClusterVersion(ctx context.Context, cluster *operatorv1alpha1.Cluster) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	newPhase := cluster.Status.Phase
	if cluster.Status.Phase != "Paused" && cluster.Status.Phase != "Deploying" {
		newPhase = "Deployed"
	}

	if cluster.Status.CurrentVersion == cluster.Spec.Version && cluster.Status.Phase == newPhase {
		return ctrl.Result{}, nil
	}

	log.Info("Updating current version", "oldVersion", cluster.Status.CurrentVersion, "newVersion", cluster.Spec.Version, "phase", newPhase)
	patch := client.MergeFrom(cluster.DeepCopy())
	cluster.Status.CurrentVersion = cluster.Spec.Version
	cluster.Status.Phase = newPhase

	if err := r.Status().Patch(ctx, cluster, patch); err != nil {
		log.Error(err, "Failed to patch cluster status with new version")
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	return ctrl.Result{}, nil
}
