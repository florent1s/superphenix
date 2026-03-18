package cluster

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

// reconcileApplication ensures an ArgoCD Application exists for the cluster.
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
		return err
	}

	log.Info("Successfully reconciled ArgoCD Application", "Application.Name", app.GetName())
	return nil
}

func (r *Reconciler) initApplication(cluster *operatorv1alpha1.Cluster) *unstructured.Unstructured {
	app := &unstructured.Unstructured{}
	app.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "Application",
	})
	app.SetName(cluster.Name)
	app.SetNamespace(r.OperatorNamespace)
	return app
}

func (r *Reconciler) setApplicationOwnership(cluster *operatorv1alpha1.Cluster, app *unstructured.Unstructured) error {
	// Set ownership reference back to the Cluster CRD
	if err := controllerutil.SetControllerReference(cluster, app, r.Scheme); err != nil {
		return err
	}

	// Set finalizer for cascade delete
	finalizers := app.GetFinalizers()
	hasFinalizer := false
	for _, f := range finalizers {
		if f == "resources-finalizer.argocd.argoproj.io" {
			hasFinalizer = true
			break
		}
	}
	if !hasFinalizer {
		app.SetFinalizers(append(finalizers, "resources-finalizer.argocd.argoproj.io"))
	}
	return nil
}

func (r *Reconciler) buildApplicationSpec(cluster *operatorv1alpha1.Cluster) map[string]interface{} {
	return map[string]interface{}{
		"project": "default",
		"source": map[string]interface{}{
			"repoURL":        "https://github.com/super-phenix/superphenix-apps.git", // Placeholder, might need to be configurable
			"path":           "clusters/" + cluster.Name,
			"targetRevision": "HEAD",
		},
		"destination": map[string]interface{}{
			"server":    "https://kubernetes.default.svc", // Local management cluster
			"namespace": "argocd",                         // Standard ArgoCD namespace, or maybe r.OperatorNamespace?
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
