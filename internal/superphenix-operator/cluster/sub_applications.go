package cluster

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
	"github.com/super-phenix/superphenix/internal/superphenix-operator/version"
)

// syncSubApplications triggers an ArgoCD sync on the root Application and on every child
// Application that belongs to the cluster.
//
// In the app of apps pattern, the root Application deploys a Helm chart that in turn contains
// ArgoCD Application manifests. ArgoCD applies those manifests, creating child Applications that
// each manage a specific component of the Superphenix stack. This function ensures the entire
// Application tree is periodically synced so that drift is corrected without relying solely on
// ArgoCD's own internal polling interval.
//
// The mechanism is to set the operation.sync field directly on each Application object, which
// is the programmatic equivalent of running `argocd app sync`. Because operation is a top-level
// field (not under spec), patching it does not change the object's generation, so the
// GenerationChangedPredicate on the Application watch is not triggered and no reconcile loop occurs.
// Failures are non-fatal; individual errors are logged and the function continues with remaining Applications.
func (r *Reconciler) syncSubApplications(ctx context.Context, cluster *operatorv1alpha1.Cluster) {
	log := logf.FromContext(ctx)

	// Also sync the root Application itself so it re-evaluates the chart and propagates
	// any changes down to the sub-applications on the same cycle.
	rootApp, err := r.fetchApplication(ctx, cluster.Name)
	if err != nil {
		log.Error(err, "Failed to fetch root Application for sync")
	} else if err := r.triggerApplicationSync(ctx, rootApp); err != nil {
		log.Error(err, "Failed to sync root Application")
	}

	subApps, err := r.listSubApplications(ctx, cluster)
	if err != nil {
		log.Error(err, "Failed to list sub-applications, skipping periodic sync")
		return
	}

	if len(subApps) == 0 {
		log.Info("No sub-applications found for cluster", "cluster", cluster.Name)
		return
	}

	synced := 0
	for i := range subApps {
		if err := r.triggerApplicationSync(ctx, &subApps[i]); err != nil {
			log.Error(err, "Failed to sync sub-application", "name", subApps[i].GetName())
			continue
		}
		synced++
	}

	log.Info("Periodic sync triggered", "cluster", cluster.Name, "appsSynced", synced, "appsTotal", len(subApps))
}

// fetchApplication retrieves a single ArgoCD Application by name from the operator namespace.
func (r *Reconciler) fetchApplication(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	app := &unstructured.Unstructured{}
	app.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "Application",
	})

	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: r.OperatorNamespace}, app); err != nil {
		return nil, fmt.Errorf("failed to get Application %s: %w", name, err)
	}

	return app, nil
}

// listSubApplications returns all ArgoCD Applications labelled with the cluster's name.
// Sub-applications are created by ArgoCD when it applies the root Application's Helm chart.
// The chart names them as "{cluster.name}-{component}" and attaches the ClusterLabel so they
// can be listed unambiguously without colliding with sub-applications from other clusters.
func (r *Reconciler) listSubApplications(ctx context.Context, cluster *operatorv1alpha1.Cluster) ([]unstructured.Unstructured, error) {
	appList := &unstructured.UnstructuredList{}
	appList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "ApplicationList",
	})

	if err := r.List(ctx, appList,
		client.InNamespace(r.OperatorNamespace),
		client.MatchingLabels{version.ClusterLabel: cluster.Name},
	); err != nil {
		return nil, fmt.Errorf("failed to list sub-applications for cluster %s: %w", cluster.Name, err)
	}

	return appList.Items, nil
}

// triggerApplicationSync triggers an immediate sync on the given ArgoCD Application by setting
// the operation.sync field, which is the programmatic equivalent of `argocd app sync`.
// ArgoCD detects the field, executes the sync, then clears it.
// A sync is skipped if an operation is already in progress to avoid queue buildup.
func (r *Reconciler) triggerApplicationSync(ctx context.Context, app *unstructured.Unstructured) error {
	log := logf.FromContext(ctx)

	// Don't start a new sync if one is already running.
	phase, _, _ := unstructured.NestedString(app.Object, "status", "operationState", "phase")
	if phase == "Running" {
		log.Info("Application already syncing, skipping trigger", "name", app.GetName())
		return nil
	}

	patch := client.MergeFrom(app.DeepCopy())

	if err := unstructured.SetNestedMap(app.Object, map[string]interface{}{
		"sync": map[string]interface{}{},
	}, "operation"); err != nil {
		return fmt.Errorf("failed to build sync operation for Application %s: %w", app.GetName(), err)
	}

	if err := r.Patch(ctx, app, patch); err != nil {
		return fmt.Errorf("failed to trigger sync on Application %s: %w", app.GetName(), err)
	}

	log.Info("Sync operation triggered on Application", "name", app.GetName())
	return nil
}
