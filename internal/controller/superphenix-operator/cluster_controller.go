package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

const (
	FinalizerName = "operator.superphenix.net/finalizer"
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
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
	// Verify the cluster can be reached and administered
	if result, err := r.reconcileHealth(ctx, cluster); err != nil || !result.IsZero() {
		return result, err
	}

	// Reconcile again in 5 minutes to ensure the cluster stays in sync
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// reconcileHealth checks the connectivity of the cluster (local or remote) and updates its status.
// If the cluster is unreachable, it returns a Requeue result.
func (r *ClusterReconciler) reconcileHealth(ctx context.Context, cluster *operatorv1alpha1.Cluster) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var config *rest.Config
	var err error

	config, err = r.getRESTConfigForCluster(ctx, cluster)
	if err != nil {
		log.Error(err, "Failed to build REST config for cluster")
		// The status condition is already set inside getRESTConfigForCluster
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if err := r.checkReachability(config); err != nil {
		log.Error(err, "Cluster unreachable")
		r.updateStatus(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, operatorv1alpha1.ReasonConnectionFailed, err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	log.Info("Cluster is reachable")
	r.updateStatus(ctx, cluster, operatorv1alpha1.ConditionTypeConnected, metav1.ConditionTrue, operatorv1alpha1.ReasonConnectionSuccess, "Successfully connected to cluster")

	return ctrl.Result{}, nil
}

// getRESTConfigForCluster generates the configuration to connect to a Kubernetes cluster
func (r *ClusterReconciler) getRESTConfigForCluster(ctx context.Context, cluster *operatorv1alpha1.Cluster) (*rest.Config, error) {
	log := logf.FromContext(ctx)

	if cluster.Spec.Connection == nil {
		err := fmt.Errorf("%w: connection configuration is missing", errConfig)
		r.updateStatus(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, operatorv1alpha1.ReasonConnectionConfigError, err.Error())
		return nil, err
	}

	if cluster.Spec.Connection.Mode == operatorv1alpha1.ConnectionModeLocal {
		log.Info("Using local connection mode")
		config, err := ctrl.GetConfig()
		if err != nil {
			r.updateStatus(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, operatorv1alpha1.ReasonConnectionFailed, err.Error())
			return nil, err
		}
		return config, nil
	}

	// For remote mode, we need URL and SecretRef
	if cluster.Spec.Connection.URL == "" {
		err := fmt.Errorf("%w: connection URL must be provided in Remote mode", errConfig)
		r.updateStatus(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, operatorv1alpha1.ReasonConnectionConfigError, err.Error())
		return nil, err
	}
	if cluster.Spec.Connection.SecretRef == nil {
		err := fmt.Errorf("%w: secret reference must be provided in Remote mode", errConfig)
		r.updateStatus(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, operatorv1alpha1.ReasonConnectionConfigError, err.Error())
		return nil, err
	}

	// Fetch the Secret
	secretName := cluster.Spec.Connection.SecretRef.Name
	secretNamespace := cluster.Spec.Connection.SecretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = cluster.Namespace
	}

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret)
	if err != nil {
		reason := operatorv1alpha1.ReasonConnectionFailed
		if apierrors.IsNotFound(err) {
			reason = operatorv1alpha1.ReasonSecretNotFound
		}
		r.updateStatus(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, reason, err.Error())
		return nil, err
	}

	log.Info("Successfully fetched secret", "Secret.Name", secret.Name, "Secret.Namespace", secret.Namespace)

	// Build REST config from secret
	config, err := r.buildRESTConfig(cluster.Spec.Connection.URL, secret)
	if err != nil {
		reason := operatorv1alpha1.ReasonConnectionFailed
		if isInvalidSecretError(err) {
			reason = operatorv1alpha1.ReasonInvalidSecret
		}
		r.updateStatus(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, reason, err.Error())
		return nil, err
	}
	return config, nil
}

func (r *ClusterReconciler) buildRESTConfig(url string, secret *corev1.Secret) (*rest.Config, error) {
	config := &rest.Config{
		Host: url,
	}

	hasAuth := false

	// Basic Auth
	if username, ok := secret.Data["username"]; ok {
		config.Username = string(username)
		hasAuth = true
	}
	if password, ok := secret.Data["password"]; ok {
		config.Password = string(password)
		hasAuth = true
	}

	// Bearer Token
	if token, ok := secret.Data["bearerToken"]; ok {
		config.BearerToken = string(token)
		hasAuth = true
	}

	if !hasAuth {
		return nil, fmt.Errorf("%w: secret must contain authentication credentials (username/password or bearerToken)", errInvalidSecret)
	}

	// TLS Config
	tlsConfig := rest.TLSClientConfig{}
	if caData, ok := secret.Data["caData"]; ok {
		tlsConfig.CAData = caData
	}
	if certData, ok := secret.Data["certData"]; ok {
		tlsConfig.CertData = certData
	}
	if keyData, ok := secret.Data["keyData"]; ok {
		tlsConfig.KeyData = keyData
	}
	if insecure, ok := secret.Data["insecure"]; ok {
		tlsConfig.Insecure = string(insecure) == "true"
	}
	if serverName, ok := secret.Data["serverName"]; ok {
		tlsConfig.ServerName = string(serverName)
	}
	config.TLSClientConfig = tlsConfig

	return config, nil
}

func (r *ClusterReconciler) checkReachability(config *rest.Config) error {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return err
	}

	_, err = discoveryClient.ServerVersion()
	if err != nil {
		return err
	}

	return nil
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

var (
	errConfig        = fmt.Errorf("configuration error")
	errInvalidSecret = fmt.Errorf("invalid secret")
)

func isConfigError(err error) bool {
	return errors.Is(err, errConfig)
}

func isInvalidSecretError(err error) bool {
	return errors.Is(err, errInvalidSecret)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.Cluster{}).
		Named("cluster").
		Complete(r)
}
