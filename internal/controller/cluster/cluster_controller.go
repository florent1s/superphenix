package cluster

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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

// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=argoproj.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="*",resources="*",verbs="*"

// Reconciler reconciles a Cluster object.
type Reconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	OperatorNamespace string
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.Cluster{}, builder.WithPredicates(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Only reconcile if the generation has changed
				// (e.g. spec changes, labels, annotations)
				// This avoids reconciliation loops when the status is updated.
				return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
			},
		})).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findClustersForSecret),
		).
		Named("cluster").
		Complete(r)
}

func (r *Reconciler) findClustersForSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	clusterList := &operatorv1alpha1.ClusterList{}
	err := r.List(ctx, clusterList)
	if err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, cluster := range clusterList.Items {
		// Only consider clusters that are in the namespace of the controller
		if r.OperatorNamespace != "" && cluster.Namespace != r.OperatorNamespace {
			continue
		}

		if cluster.Spec.Connection != nil && cluster.Spec.Connection.SecretRef != nil {
			secretName := cluster.Spec.Connection.SecretRef.Name
			secretNamespace := cluster.Spec.Connection.SecretRef.Namespace
			if secretNamespace == "" {
				secretNamespace = cluster.Namespace
			}

			if secretName == secret.GetName() && secretNamespace == secret.GetNamespace() {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      cluster.Name,
						Namespace: cluster.Namespace,
					},
				})
			}
		}
	}
	return requests
}

// Reconcile is used to reconcile the state of Superphenix clusters with their definition.
// It is called on creations, updates, deletions, and re-queuing.
// This function handles retrieving the cluster object and the finalizer logic.
// It then defers the actual reconciliation/cleanup to other functions.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Only sync clusters that are in the namespace of the controller
	if r.OperatorNamespace != "" && req.Namespace != r.OperatorNamespace {
		return ctrl.Result{}, nil
	}

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
	if !cluster.ObjectMeta.DeletionTimestamp.IsZero() {
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

	// Add the finalizer if it doesn't exist to prevent the cluster from being deleted without cleaning up
	if !controllerutil.ContainsFinalizer(cluster, FinalizerName) {
		controllerutil.AddFinalizer(cluster, FinalizerName)
		if err := r.Update(ctx, cluster); err != nil {
			return ctrl.Result{RequeueAfter: time.Minute}, err
		}
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
	//if cluster.Status.Phase == "" || cluster.Status.Phase == "Deployed" {
	//	r.updateStatusWithPhase(ctx, cluster, "Ready", metav1.ConditionFalse, "Reconciling", "Reconciliation in progress", "Deploying")
	//}

	// Reconcile ArgoCD connection secret
	if err := r.reconcileArgoCDSecret(ctx, cluster); err != nil {
		log.Error(err, "Failed to reconcile ArgoCD connection secret")
		// The status condition and phase Error are set inside reconcileArgoCDSecret
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Verify the cluster can be reached and administered
	k8sVersion, result, err := r.reconcileHealth(ctx, cluster)
	if err != nil || !result.IsZero() {
		if err != nil {
			r.updateStatusWithPhase(ctx, cluster, "Ready", metav1.ConditionFalse, "HealthCheckFailed", err.Error(), "Error")
		}
		return result, err
	}

	// Update Kubernetes version in status if it changed
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
