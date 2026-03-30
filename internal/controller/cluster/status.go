package cluster

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	patch := client.MergeFrom(cluster.DeepCopy())

	if phase != "" {
		cluster.Status.Phase = phase
	}

	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: cluster.Generation,
	}

	// Ensure opposite conditions are updated too
	r.setCondition(&cluster.Status.Conditions, condition)

	cluster.Status.ObservedGeneration = cluster.Generation

	if err := r.Status().Patch(ctx, cluster, patch); err != nil {
		log.Error(err, "Failed to patch Cluster status")
	}
}

// updateKubernetesVersion updates the Kubernetes version in the cluster status if it changed.
func (r *Reconciler) updateKubernetesVersion(ctx context.Context, cluster *operatorv1alpha1.Cluster, k8sVersion string) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if cluster.Status.KubernetesVersion != k8sVersion {
		log.Info("Updating Kubernetes version", "oldVersion", cluster.Status.KubernetesVersion, "newVersion", k8sVersion)
		// Refresh object to avoid conflict
		latest := &operatorv1alpha1.Cluster{}
		if err := r.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, latest); err == nil {
			latest.Status.KubernetesVersion = k8sVersion
			if err := r.Status().Update(ctx, latest); err != nil {
				log.Error(err, "Failed to update cluster status with kubernetes version")
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
			// Update the local object as well
			cluster.Status.KubernetesVersion = k8sVersion
		}
	}
	return ctrl.Result{}, nil
}

// updateClusterVersion updates the current version and phase in the cluster status if they changed.
func (r *Reconciler) updateClusterVersion(ctx context.Context, cluster *operatorv1alpha1.Cluster) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if cluster.Status.CurrentVersion != cluster.Spec.Version {
		log.Info("Updating current version", "oldVersion", cluster.Status.CurrentVersion, "newVersion", cluster.Spec.Version)
		// Refresh object to avoid conflict
		latest := &operatorv1alpha1.Cluster{}
		if err := r.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, latest); err == nil {
			latest.Status.CurrentVersion = cluster.Spec.Version
			if cluster.Status.Phase != "Paused" && cluster.Status.Phase != "Deploying" {
				latest.Status.Phase = "Deployed"
			}
			if err := r.Status().Update(ctx, latest); err != nil {
				log.Error(err, "Failed to update cluster status with new version")
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
			// Update the local object as well so following logic sees the change
			cluster.Status.CurrentVersion = cluster.Spec.Version
			if cluster.Status.Phase != "Paused" && cluster.Status.Phase != "Deploying" {
				cluster.Status.Phase = "Deployed"
			}
		} else {
			cluster.Status.CurrentVersion = cluster.Spec.Version
			if cluster.Status.Phase != "Paused" && cluster.Status.Phase != "Deploying" {
				cluster.Status.Phase = "Deployed"
			}
			if err := r.Status().Update(ctx, cluster); err != nil {
				log.Error(err, "Failed to update cluster status with new version")
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
		}
	}

	return ctrl.Result{}, nil
}
