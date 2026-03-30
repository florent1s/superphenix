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
)

// syncSubApplications triggers a refresh on the root Application and on every child Application
// that belongs to the cluster.
//
// In the app of apps pattern, the root Application deploys a Helm chart that in turn contains
// ArgoCD Application manifests. ArgoCD applies those manifests, creating child Applications that
// each manage a specific component of the Superphenix stack. This function ensures the entire
// Application tree is periodically re-evaluated so that ArgoCD detects and corrects drift without
// relying solely on its own internal polling interval.
//
// The mechanism is a refresh annotation: ArgoCD removes it once processed, so re-adding it on
// every reconcile cycle (every 5 minutes) acts as a heartbeat-driven sync trigger. Failures are
// non-fatal; individual errors are logged and the function continues with remaining Applications.
func (r *Reconciler) syncSubApplications(ctx context.Context, cluster *operatorv1alpha1.Cluster) {
	log := logf.FromContext(ctx)

	// Also refresh the root Application itself so it re-evaluates the chart and propagates
	// any changes down to the sub-applications on the same cycle.
	rootApp, err := r.fetchApplication(ctx, cluster.Name)
	if err != nil {
		log.Error(err, "Failed to fetch root Application for refresh")
	} else if err := r.refreshApplication(ctx, rootApp); err != nil {
		log.Error(err, "Failed to refresh root Application")
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

	refreshed := 0
	for i := range subApps {
		if err := r.refreshApplication(ctx, &subApps[i]); err != nil {
			log.Error(err, "Failed to refresh sub-application", "name", subApps[i].GetName())
			continue
		}
		refreshed++
	}

	log.Info("Periodic sync triggered", "cluster", cluster.Name, "subAppsRefreshed", refreshed, "subAppsTotal", len(subApps))
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
		client.MatchingLabels{ClusterLabel: cluster.Name},
	); err != nil {
		return nil, fmt.Errorf("failed to list sub-applications for cluster %s: %w", cluster.Name, err)
	}

	return appList.Items, nil
}

// refreshApplication patches the ArgoCD refresh annotation onto the given Application.
// ArgoCD's controller picks up the annotation, re-evaluates the Application's desired state,
// and, because Applications are deployed with selfHeal:true, automatically syncs if out of sync.
// ArgoCD removes the annotation after processing, making it safe to re-apply on the next cycle.
func (r *Reconciler) refreshApplication(ctx context.Context, app *unstructured.Unstructured) error {
	patch := client.MergeFrom(app.DeepCopy())

	annotations := app.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["argocd.argoproj.io/refresh"] = "normal"
	app.SetAnnotations(annotations)

	if err := r.Patch(ctx, app, patch); err != nil {
		return fmt.Errorf("failed to patch refresh annotation on Application %s: %w", app.GetName(), err)
	}

	logf.FromContext(ctx).Info("Refresh annotation set on Application", "name", app.GetName())
	return nil
}
