package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

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

	if cluster.Spec.PauseSync {
		r.updateStatusWithPhase(ctx, cluster, operatorv1alpha1.ConditionTypePaused, metav1.ConditionTrue, operatorv1alpha1.ReasonPaused, "Synchronization is paused", "Paused")
	} else {
		// Propagate ArgoCD Application status to Cluster status
		r.propagateApplicationStatus(ctx, cluster, app)
	}

	return nil
}

// propagateApplicationStatus propagates the ArgoCD Application status to the Cluster status.
func (r *Reconciler) propagateApplicationStatus(ctx context.Context, cluster *operatorv1alpha1.Cluster, app *unstructured.Unstructured) {
	healthStatus, _, _ := unstructured.NestedString(app.Object, "status", "health", "status")
	syncStatus, _, _ := unstructured.NestedString(app.Object, "status", "sync", "status")

	var status metav1.ConditionStatus
	var reason string
	var message string
	var phase string

	switch syncStatus {
	case "Synced":
		status = metav1.ConditionTrue
		reason = operatorv1alpha1.ReasonArgoCDSynced
		message = "ArgoCD Application is synced"
		phase = "Deployed"
	case "OutOfSync":
		status = metav1.ConditionFalse
		reason = operatorv1alpha1.ReasonArgoCDOutOfSync
		message = "ArgoCD Application is out of sync"
		phase = "OutOfSync"
	case "Unknown":
		status = metav1.ConditionUnknown
		reason = operatorv1alpha1.ReasonArgoCDUnknown
		message = "ArgoCD Application status is unknown"
		phase = "Unknown"
	case "Syncing":
		status = metav1.ConditionFalse
		reason = operatorv1alpha1.ReasonArgoCDSyncing
		message = "ArgoCD Application is syncing"
		phase = "Deploying"
	default:
		status = metav1.ConditionFalse
		reason = operatorv1alpha1.ReasonArgoCDSyncFailed
		message = fmt.Sprintf("ArgoCD Application sync status: %s", syncStatus)
		phase = "Error"
	}

	if healthStatus == "Degraded" {
		phase = "Error"
		message = fmt.Sprintf("%s (Health: %s)", message, healthStatus)
		status = metav1.ConditionFalse
		reason = operatorv1alpha1.ReasonHealthCheckFailed
	} else if healthStatus == "Progressing" {
		phase = "Deploying"
		message = fmt.Sprintf("%s (Health: %s)", message, healthStatus)
		if syncStatus == "Synced" {
			status = metav1.ConditionFalse
			reason = operatorv1alpha1.ReasonArgoCDSyncing
		}
	} else if healthStatus == "Suspended" || healthStatus == "Missing" {
		phase = "Unknown"
		message = fmt.Sprintf("%s (Health: %s)", message, healthStatus)
		status = metav1.ConditionUnknown
	}

	r.updateStatusWithPhase(ctx, cluster, operatorv1alpha1.ConditionTypeArgoCDSynced, status, reason, message, phase)

	// Update Ready condition based on both Reachable and ArgoCDSynced
	readyStatus := metav1.ConditionTrue
	readyReason := operatorv1alpha1.ReasonReconcileSuccess
	readyMessage := "Cluster is ready"

	reachable := false
	for _, c := range cluster.Status.Conditions {
		if c.Type == operatorv1alpha1.ConditionTypeReachable && c.Status == metav1.ConditionTrue {
			reachable = true
			break
		}
	}

	if !reachable {
		readyStatus = metav1.ConditionFalse
		readyReason = operatorv1alpha1.ReasonConnectionFailed
		readyMessage = "Cluster is unreachable"
	} else if status != metav1.ConditionTrue {
		readyStatus = status
		readyReason = reason
		readyMessage = message
	}

	r.updateStatus(ctx, cluster, operatorv1alpha1.ConditionTypeReady, readyStatus, readyReason, readyMessage)
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

	repoURL := r.DefaultRepoURL
	if cluster.Spec.RepoURL != "" {
		repoURL = cluster.Spec.RepoURL
	}

	chartName := r.DefaultChartName
	if cluster.Spec.ChartName != "" {
		chartName = cluster.Spec.ChartName
	}

	targetRevision := r.DefaultVersion
	if cluster.Spec.Version != "" {
		targetRevision = cluster.Spec.Version
	}

	syncPolicy := map[string]interface{}{
		"syncOptions": []interface{}{
			"CreateNamespace=true",
			"PrunePropagationPolicy=foreground",
			"PruneLast=true",
			"SkipDryRunOnMissingResource=true",
		},
	}

	if !cluster.Spec.PauseSync {
		syncPolicy["automated"] = map[string]interface{}{
			"prune":    true,
			"selfHeal": true,
		}
	}

	return map[string]interface{}{
		"project": cluster.Name,
		"source": map[string]interface{}{
			"repoURL":        repoURL,
			"chart":          chartName,
			"targetRevision": targetRevision,
			"helm": map[string]interface{}{
				"valuesObject": r.generateApplicationValues(cluster),
			},
		},
		"destination": map[string]interface{}{
			"name":      destName,
			"namespace": r.OperatorNamespace,
		},
		"syncPolicy": syncPolicy,
	}
}

// generateApplicationValues generates the values for the cluster application Helm chart.
func (r *Reconciler) generateApplicationValues(cluster *operatorv1alpha1.Cluster) map[string]interface{} {
	values := map[string]interface{}{
		"cluster": map[string]interface{}{
			"name":             cluster.Name,
			"region":           cluster.Spec.Region,
			"availabilityZone": cluster.Spec.AvailabilityZone,
			"deploymentMode":   string(cluster.Spec.DeploymentMode),
		},
		"argocd": map[string]interface{}{
			"namespace": r.OperatorNamespace,
			"project":   cluster.Name,
		},
	}

	if cluster.Spec.Type != nil {
		values["cluster"].(map[string]interface{})["type"] = string(*cluster.Spec.Type)
	}

	version := r.DefaultVersion
	if cluster.Spec.Version != "" {
		version = cluster.Spec.Version
	}
	values["cluster"].(map[string]interface{})["version"] = version

	if cluster.Spec.SystemConfiguration != nil {
		var systemConfig map[string]interface{}
		if err := json.Unmarshal(cluster.Spec.SystemConfiguration.Raw, &systemConfig); err == nil {
			maps.Copy(values, systemConfig)
		}
	}

	return values
}
