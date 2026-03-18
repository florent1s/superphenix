package cluster

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

// reconcileApplication ensures an ArgoCD Application exists for each cluster.
// This application is used as the root of all the deployments done on each cluster.
// It uses the App of Apps pattern to deploy in cascade the entire Superphenix stack.
func (r *Reconciler) reconcileApplication(ctx context.Context, cluster *operatorv1alpha1.Cluster) error {
	log := logf.FromContext(ctx)

	app := r.initApplication(cluster)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, app, func() error {
		// Set ownership and finalizers
		if err := r.setApplicationOwnership(cluster, app); err != nil {
			return err
		}

		// Define and set Application Spec
		spec := r.buildApplicationSpec(cluster)
		return unstructured.SetNestedMap(app.Object, spec, "spec")
	})

	if err != nil {
		log.Error(err, "Failed to reconcile ArgoCD Application")
		r.updateStatusWithPhase(ctx, cluster, operatorv1alpha1.ConditionTypeReady, metav1.ConditionFalse, operatorv1alpha1.ReasonApplicationReconcileFailed, err.Error(), "Error")
		return err
	}

	log.Info("Successfully reconciled ArgoCD Application", "Application.Name", app.GetName())
	return nil
}

// initApplication creates the template of the cluster application.
func (r *Reconciler) initApplication(cluster *operatorv1alpha1.Cluster) *unstructured.Unstructured {
	app := &unstructured.Unstructured{}
	app.SetName(cluster.Name)
	app.SetNamespace(r.OperatorNamespace)
	app.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "Application",
	})

	// The application will clean up its children's resources on deletion using this finalizer
	app.SetFinalizers([]string{"resources-finalizer.argocd.argoproj.io"})

	return app
}

// setApplicationOwnership ensures the application is owned by the Cluster CRD.
func (r *Reconciler) setApplicationOwnership(cluster *operatorv1alpha1.Cluster, app *unstructured.Unstructured) error {
	// Set the ownership reference back to the Cluster CRD
	if err := controllerutil.SetControllerReference(cluster, app, r.Scheme); err != nil {
		return err
	}

	return nil
}

// buildApplicationSpec creates the specs of the cluster application.
func (r *Reconciler) buildApplicationSpec(cluster *operatorv1alpha1.Cluster) map[string]interface{} {
	destName := cluster.Name
	if cluster.Spec.Connection.Mode == operatorv1alpha1.ConnectionModeLocal {
		destName = "in-cluster"
	}

	return map[string]interface{}{
		"project": cluster.Name,
		"source": map[string]interface{}{
			"repoURL":        "git@github.com:super-phenix/superphenix.git",
			"path":           "deployments/charts/superphenix",
			"targetRevision": "HEAD",
			"helm": map[string]interface{}{
				"valuesObject": r.generateApplicationValues(cluster),
			},
		},
		"destination": map[string]interface{}{
			"name":      destName,
			"namespace": r.OperatorNamespace,
		},
		"syncPolicy": map[string]interface{}{
			"automated": map[string]interface{}{
				"prune":    true,
				"selfHeal": true,
			},
			"syncOptions": []interface{}{
				"CreateNamespace=true",
				"PrunePropagationPolicy=foreground",
				"PruneLast=true",
			},
		},
	}
}

// generateApplicationValues generates the values for the cluster application Helm chart.
func (r *Reconciler) generateApplicationValues(cluster *operatorv1alpha1.Cluster) map[string]interface{} {
	return map[string]interface{}{}
}
