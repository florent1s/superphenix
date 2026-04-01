package cluster

import (
	"context"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

const (
	// FinalizerName is the name of the finalizer used to clean up the cluster when it is deleted.
	FinalizerName = "operator.superphenix.net/finalizer"
	// ClusterLabel is the label used to identify the cluster in ArgoCD.
	ClusterLabel = "operator.superphenix.net/clusterName"
)

var (
	errConfig        = fmt.Errorf("configuration error")
	errInvalidSecret = fmt.Errorf("invalid secret")
)

// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=argoproj.io,resources=applications;appprojects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="*",resources="*",verbs="*"

// Reconciler reconciles a Cluster object.
type Reconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	OperatorNamespace string
	DefaultRepoURL    string
	DefaultChartName  string
	DefaultVersion    string
	// ArgoCDApplicationWatchStarted is true if the watch for ArgoCD Applications has been started.
	ArgoCDApplicationWatchStarted bool
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
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
		)

	c, err := b.Named("cluster").Build(r)
	if err != nil {
		return err
	}

	// Start a background routine to watch for ArgoCD Application CRD
	go r.watchArgoCDApplications(mgr, c)

	return nil
}

func (r *Reconciler) watchArgoCDApplications(mgr ctrl.Manager, c controller.Controller) {
	ctx := context.Background()
	log := logf.Log.WithName("cluster").WithName("argocd-watch")

	// Wait for cache to sync before checking CRD
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		log.Error(nil, "Failed to sync cache")
		return
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	timeout := time.After(5 * time.Minute)

	for {
		if _, err := mgr.GetRESTMapper().RESTMapping(schema.GroupKind{Group: "argoproj.io", Kind: "Application"}); err == nil {
			log.Info("ArgoCD Application CRD found, starting watch")
			err := c.Watch(
				source.Kind[client.Object](mgr.GetCache(), &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "argoproj.io/v1alpha1",
						"kind":       "Application",
					},
				},
					handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &operatorv1alpha1.Cluster{}),
					predicate.GenerationChangedPredicate{},
				),
			)
			if err != nil {
				log.Error(err, "Failed to start watch for ArgoCD Applications")
				// We'll retry on the next tick
			} else {
				r.ArgoCDApplicationWatchStarted = true
				log.Info("Successfully started watch for ArgoCD Applications")
				return
			}
		} else {
			log.Info("ArgoCD Application CRD not yet available, retrying in 10s...")
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Continue loop
		case <-timeout:
			log.Error(nil, "ArgoCD Application CRD not found after 5 minutes, crashing")
			os.Exit(1)
		}
	}
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

	// Check if ArgoCD CRDs are installed
	if err := r.checkArgoCDCRDs(ctx); err != nil {
		r.updateArgoCDCondition(ctx, cluster, metav1.ConditionFalse, operatorv1alpha1.ReasonArgoCDCRDMissing, err.Error())
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	r.updateArgoCDCondition(ctx, cluster, metav1.ConditionTrue, operatorv1alpha1.ReasonArgoCDCRDInstalled, "ArgoCD CRDs are installed")

	// Handle the reconciling logic for the cluster
	res, err := r.reconcileCluster(ctx, cluster)

	// If the ArgoCD Application watch wasn't set up (e.g. CRDs were missing at startup),
	// we should requeue more frequently to see if it's there now.
	// However, we already have a checkArgoCDCRDs in Reconcile which handles this.
	return res, err
}

// checkArgoCDCRDs verifies if the required ArgoCD CRDs are installed in the management cluster.
func (r *Reconciler) checkArgoCDCRDs(ctx context.Context) error {
	log := logf.FromContext(ctx)

	gvks := []schema.GroupVersionKind{
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "Application"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "AppProject"},
	}

	for _, gvk := range gvks {
		_, err := r.RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			if meta.IsNoMatchError(err) {
				log.Info("ArgoCD CRD not found", "GVK", gvk.String())
				return fmt.Errorf("ArgoCD CRD %s not found", gvk.Kind)
			}
			return err
		}
	}

	return nil
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
	k8sVersion, result, err := r.reconcileHealth(ctx, cluster)
	if err != nil || !result.IsZero() {
		if err != nil {
			r.updateStatusWithPhase(ctx, cluster, operatorv1alpha1.ConditionTypeReady, metav1.ConditionFalse, operatorv1alpha1.ReasonHealthCheckFailed, err.Error(), "Error")
		}

		// Even if the cluster is unreachable, we still try to reconcile the ArgoCD AppProject and Application
		// so that they are created/updated with the correct destination.
		// This is useful when the cluster is not yet reachable but we want to prepare the ArgoCD resources.
		_ = r.reconcileAppProject(ctx, cluster)
		_ = r.reconcileApplication(ctx, cluster)

		return result, err
	}

	// Update the Kubernetes version in status if it changed
	if result, err := r.updateKubernetesVersion(ctx, cluster, k8sVersion); err != nil || !result.IsZero() {
		return result, err
	}

	// Validate cluster configuration
	if err := r.validate(ctx, cluster); err != nil {
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Reconcile ArgoCD AppProject
	if err := r.reconcileAppProject(ctx, cluster); err != nil {
		log.Error(err, "Failed to reconcile ArgoCD AppProject")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Reconcile ArgoCD Application
	if err := r.reconcileApplication(ctx, cluster); err != nil {
		log.Error(err, "Failed to reconcile ArgoCD Application")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Trigger a periodic refresh on every sub-application within the app of apps chart.
	// This keeps child Applications in sync without relying solely on ArgoCD's internal polling.
	if !cluster.Spec.PauseSync {
		r.syncSubApplications(ctx, cluster)
	}

	// Update the current version and phase in status if everything else is healthy
	if result, err := r.updateClusterVersion(ctx, cluster); err != nil || !result.IsZero() {
		return result, err
	}

	// Reconcile again in 5 minutes to ensure the cluster stays in sync
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *Reconciler) updateArgoCDCondition(ctx context.Context, cluster *operatorv1alpha1.Cluster, status metav1.ConditionStatus, reason, message string) {
	r.updateStatus(ctx, cluster, operatorv1alpha1.ConditionTypeArgoCDInstalled, status, reason, message)
}
