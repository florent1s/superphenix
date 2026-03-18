package cluster

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

const (
	// FinalizerName is the name of the finalizer used to clean up the cluster when it is deleted.
	FinalizerName = "operator.superphenix.net/finalizer"
	// ClusterLabel is the label used to identify the cluster in ArgoCD.
	ClusterLabel = "operator.superphenix.net/cluster-name"
)

var (
	errConfig        = fmt.Errorf("configuration error")
	errInvalidSecret = fmt.Errorf("invalid secret")
)

// Reconciler reconciles a Cluster object.
type Reconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	OperatorNamespace string
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.Cluster{}).
		Named("cluster").
		Complete(r)
}

// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=argoproj.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="*",resources="*",verbs="*"

// Reconcile is used to reconcile the state of Superphenix clusters with their definition.
// It is called on creations, updates, deletions, and re-queuing.
// This function handles retrieving the cluster object and the finalizer logic.
// It then defers the actual reconciliation/cleanup to other functions.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Fetch the Cluster instance
	cluster := &operatorv1alpha1.Cluster{}
	err := r.Get(ctx, req.NamespacedName, cluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Examine DeletionTimestamp to determine if the cluster is under deletion
	if cluster.ObjectMeta.DeletionTimestamp.IsZero() {
		// Add the finalizer if it doesn't exist to prevent the cluster from being deleted without cleaning up
		if !controllerutil.ContainsFinalizer(cluster, FinalizerName) {
			controllerutil.AddFinalizer(cluster, FinalizerName)
			if err := r.Update(ctx, cluster); err != nil {
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
		}
	} else {
		// The cluster is being deleted and the finalizer is present, so clean it up
		if controllerutil.ContainsFinalizer(cluster, FinalizerName) {
			if err := r.cleanupCluster(ctx, cluster); err != nil {
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}

			controllerutil.RemoveFinalizer(cluster, FinalizerName)
			if err := r.Update(ctx, cluster); err != nil {
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
		}

		return ctrl.Result{}, nil
	}

	// Handle the reconciling logic for the cluster
	return r.reconcileCluster(ctx, cluster)
}

// reconcileCluster checks if the cluster can be reached and administered and then deploys
// the Superphenix stack on it. The logic is run every 5 minutes to address runtime drifts
// and re-check if the cluster can still be reached.
func (r *Reconciler) reconcileCluster(ctx context.Context, cluster *operatorv1alpha1.Cluster) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Set phase to Deploying at the start of reconciliation
	if cluster.Status.Phase == "" || cluster.Status.Phase == "Deployed" {
		r.updateStatusWithPhase(ctx, cluster, "Ready", metav1.ConditionFalse, "Reconciling", "Reconciliation in progress", "Deploying")
	}

	// Reconcile ArgoCD connection secret
	if err := r.reconcileArgoCDSecret(ctx, cluster); err != nil {
		log.Error(err, "Failed to reconcile ArgoCD connection secret")
		// The status condition and phase Error are set inside reconcileArgoCDSecret
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Verify the cluster can be reached and administered
	result, err := r.reconcileHealth(ctx, cluster)
	if err != nil || !result.IsZero() {
		if err != nil {
			r.updateStatusWithPhase(ctx, cluster, "Ready", metav1.ConditionFalse, "HealthCheckFailed", err.Error(), "Error")
		}
		return result, err
	}

	// Validate cluster configuration
	if err := r.validate(ctx, cluster); err != nil {
		log.Error(err, "Validation failed")
		r.updateStatusWithPhase(ctx, cluster, "Ready", metav1.ConditionFalse, operatorv1alpha1.ReasonInvalidVersion, err.Error(), "Error")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Reconcile ArgoCD Application
	if err := r.reconcileApplication(ctx, cluster); err != nil {
		log.Error(err, "Failed to reconcile ArgoCD Application")
		r.updateStatusWithPhase(ctx, cluster, "Ready", metav1.ConditionFalse, "ApplicationReconcileFailed", err.Error(), "Error")
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	// Update the current version in status if everything else is healthy
	if cluster.Status.CurrentVersion != cluster.Spec.Version {
		log.Info("Updating current version", "oldVersion", cluster.Status.CurrentVersion, "newVersion", cluster.Spec.Version)
		// Refresh object to avoid conflict
		latest := &operatorv1alpha1.Cluster{}
		if err := r.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, latest); err == nil {
			latest.Status.CurrentVersion = cluster.Spec.Version
			latest.Status.Phase = "Deployed"
			if err := r.Status().Update(ctx, latest); err != nil {
				log.Error(err, "Failed to update cluster status with new version")
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
			// Update the local object as well so following logic sees the change
			cluster.Status.CurrentVersion = cluster.Spec.Version
			cluster.Status.Phase = "Deployed"
		} else {
			cluster.Status.CurrentVersion = cluster.Spec.Version
			cluster.Status.Phase = "Deployed"
			if err := r.Status().Update(ctx, cluster); err != nil {
				log.Error(err, "Failed to update cluster status with new version")
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
		}
	} else {
		// If version is already correct and we reached here, it's Deployed
		if cluster.Status.Phase != "Deployed" {
			r.updateStatusWithPhase(ctx, cluster, "Ready", metav1.ConditionTrue, "ReconcileSuccess", "Cluster is fully reconciled", "Deployed")
		}
	}

	// Reconcile again in 5 minutes to ensure the cluster stays in sync
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}
