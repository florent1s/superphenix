package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
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

// ClusterReconciler reconciles a Cluster object.
type ClusterReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	OperatorNamespace string
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.Cluster{}).
		Named("cluster").
		Complete(r)
}

// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="*",resources="*",verbs="*"

// Reconcile is used to reconcile the state of Superphenix clusters with their definition.
// It is called on creations, updates, deletions, and re-queuing.
// This function handles retrieving the cluster object and the finalizer logic.
// It then defers the actual reconciliation to another function.
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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
func (r *ClusterReconciler) reconcileCluster(ctx context.Context, cluster *operatorv1alpha1.Cluster) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Reconcile ArgoCD connection secret
	if err := r.reconcileArgoCDSecret(ctx, cluster); err != nil {
		log.Error(err, "Failed to reconcile ArgoCD connection secret")
		// The status condition is already set inside reconcileArgoCDSecret
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Verify the cluster can be reached and administered
	result, err := r.reconcileHealth(ctx, cluster)
	if err != nil || !result.IsZero() {
		return result, err
	}

	// Validate version upgrade/downgrade
	if err := r.validateUpgradePath(ctx, cluster); err != nil {
		log.Error(err, "Invalid version change")
		r.updateStatus(ctx, cluster, "Ready", metav1.ConditionFalse, operatorv1alpha1.ReasonInvalidVersion, err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Update current version in status if everything else is healthy
	if cluster.Status.CurrentVersion != cluster.Spec.Version {
		log.Info("Updating current version", "oldVersion", cluster.Status.CurrentVersion, "newVersion", cluster.Spec.Version)
		// Refresh object to avoid conflict
		latest := &operatorv1alpha1.Cluster{}
		if err := r.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, latest); err == nil {
			latest.Status.CurrentVersion = cluster.Spec.Version
			if err := r.Status().Update(ctx, latest); err != nil {
				log.Error(err, "Failed to update cluster status with new version")
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
			// Update the local object as well so following logic sees the change
			cluster.Status.CurrentVersion = cluster.Spec.Version
		} else {
			cluster.Status.CurrentVersion = cluster.Spec.Version
			if err := r.Status().Update(ctx, cluster); err != nil {
				log.Error(err, "Failed to update cluster status with new version")
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
		}
	}

	// Reconcile again in 5 minutes to ensure the cluster stays in sync
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// reconcileArgoCDSecret ensures the secret used by ArgoCD to connect to the clusters is up to date
// with the connection configuration from the Cluster CRD.
func (r *ClusterReconciler) reconcileArgoCDSecret(ctx context.Context, cluster *operatorv1alpha1.Cluster) error {
	log := logf.FromContext(ctx)

	// Only populate data for remote clusters. ArgoCD will create an "in-cluster" config
	// for the local cluster on which the operator and the management ArgoCD are deployed.
	if cluster.Spec.Connection.Mode != operatorv1alpha1.ConnectionModeRemote {
		log.Info("Skipping ArgoCD secret reconciliation for local cluster", "cluster", cluster.Name)
		return nil
	}

	// The secret used by ArgoCD to connect to the cluster.
	secretName := fmt.Sprintf("cluster-%s", cluster.Name)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: r.OperatorNamespace,
		},
	}

	// Create or update the secret
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		secret.Labels["argocd.argoproj.io/secret-type"] = "cluster"
		secret.Labels[ClusterLabel] = cluster.Name

		// Set ownership reference
		if err := controllerutil.SetControllerReference(cluster, secret, r.Scheme); err != nil {
			return err
		}

		// Prepare secret data
		data := make(map[string][]byte)
		data["name"] = []byte(cluster.Name)
		data["server"] = []byte(cluster.Spec.Connection.URL)

		// Fetch the connection secret to get credentials
		if cluster.Spec.Connection.SecretRef == nil {
			return fmt.Errorf("%w: connection.secretRef is required when Mode is Remote", errConfig)
		}
		connSecretName := cluster.Spec.Connection.SecretRef.Name
		connSecretNamespace := cluster.Spec.Connection.SecretRef.Namespace
		if connSecretNamespace == "" {
			connSecretNamespace = cluster.Namespace
		}

		connSecret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: connSecretName, Namespace: connSecretNamespace}, connSecret)
		if err != nil {
			return err
		}

		connData := r.extractConnectionData(connSecret)
		if connData.bearerToken == "" && connData.username == "" && connData.password == "" {
			return fmt.Errorf("%w: secret must contain either a bearerToken or a username/password pair", errInvalidSecret)
		}

		// Build ArgoCD cluster config
		config := map[string]interface{}{}
		if connData.bearerToken != "" {
			config["bearerToken"] = connData.bearerToken
		}
		if connData.username != "" {
			config["username"] = connData.username
		}
		if connData.password != "" {
			config["password"] = connData.password
		}
		config["tlsClientConfig"] = connData.tlsClientConfig

		configBytes, err := json.Marshal(config)
		if err != nil {
			return err
		}
		data["config"] = configBytes

		secret.Data = data
		return nil
	})

	if err != nil {
		reason := operatorv1alpha1.ReasonConnectionConfigError
		if apierrors.IsNotFound(err) || (errors.Is(err, apierrors.NewNotFound(corev1.Resource("secret"), "")) || apierrors.IsNotFound(errors.Unwrap(err))) {
			reason = operatorv1alpha1.ReasonSecretNotFound
		} else if errors.Is(err, errConfig) {
			reason = operatorv1alpha1.ReasonConnectionConfigError
		} else if errors.Is(err, errInvalidSecret) {
			reason = operatorv1alpha1.ReasonInvalidSecret
		}
		r.updateStatus(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, reason, err.Error())
		return err
	}

	log.Info("Successfully reconciled ArgoCD connection secret", "Secret.Name", secretName, "Secret.Namespace", r.OperatorNamespace)
	return nil
}

type connectionData struct {
	bearerToken     string
	username        string
	password        string
	caData          []byte
	certData        []byte
	keyData         []byte
	insecure        bool
	serverName      string
	hasAuth         bool
	tlsClientConfig map[string]interface{}
}

func (r *ClusterReconciler) extractConnectionData(secret *corev1.Secret) *connectionData {
	data := &connectionData{
		tlsClientConfig: make(map[string]interface{}),
	}

	// Auth
	if token, ok := secret.Data["bearerToken"]; ok {
		data.bearerToken = string(token)
		data.hasAuth = true
	}
	if username, ok := secret.Data["username"]; ok {
		data.username = string(username)
		data.hasAuth = true
	}
	if password, ok := secret.Data["password"]; ok {
		data.password = string(password)
		data.hasAuth = true
	}

	// TLS
	if caData, ok := secret.Data["caData"]; ok {
		data.caData = caData
		data.tlsClientConfig["caData"] = string(caData)
	}
	if certData, ok := secret.Data["certData"]; ok {
		data.certData = certData
		data.tlsClientConfig["certData"] = string(certData)
	}
	if keyData, ok := secret.Data["keyData"]; ok {
		data.keyData = keyData
		data.tlsClientConfig["keyData"] = string(keyData)
	}
	if insecure, ok := secret.Data["insecure"]; ok {
		data.insecure = string(insecure) == "true"
		data.tlsClientConfig["insecure"] = data.insecure
	}
	if serverName, ok := secret.Data["serverName"]; ok {
		data.serverName = string(serverName)
		data.tlsClientConfig["serverName"] = data.serverName
	}

	return data
}

func (r *ClusterReconciler) setCondition(conditions *[]metav1.Condition, newCondition metav1.Condition) {
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

func (r *ClusterReconciler) updateStatus(ctx context.Context, cluster *operatorv1alpha1.Cluster, condType string, status metav1.ConditionStatus, reason, message string) {
	log := logf.FromContext(ctx)
	patch := client.MergeFrom(cluster.DeepCopy())
	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: cluster.Generation,
	}

	// Ensure opposite conditions are updated too
	if condType == operatorv1alpha1.ConditionTypeConnected && status == metav1.ConditionTrue {
		r.setCondition(&cluster.Status.Conditions, condition)
		r.setCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               operatorv1alpha1.ConditionTypeUnreachable,
			Status:             metav1.ConditionFalse,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: cluster.Generation,
		})
	} else if condType == operatorv1alpha1.ConditionTypeUnreachable && status == metav1.ConditionTrue {
		r.setCondition(&cluster.Status.Conditions, condition)
		r.setCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               operatorv1alpha1.ConditionTypeConnected,
			Status:             metav1.ConditionFalse,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: cluster.Generation,
		})
	} else {
		r.setCondition(&cluster.Status.Conditions, condition)
	}

	cluster.Status.ObservedGeneration = cluster.Generation

	if err := r.Status().Patch(ctx, cluster, patch); err != nil {
		log.Error(err, "Failed to patch Cluster status")
	}
}

func (r *ClusterReconciler) cleanupCluster(ctx context.Context, cluster *operatorv1alpha1.Cluster) error {
	log := logf.FromContext(ctx)
	log.Info("Cleaning up external resources for Cluster", "Name", cluster.Name)

	return nil
}
