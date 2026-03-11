package controller

import (
	"context"

	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

// ClusterConnectionReconciler reconciles a ClusterConnection object
type ClusterConnectionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusterconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusterconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.superphenix.net,resources=clusterconnections/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ClusterConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the ClusterConnection instance
	clusterConnection := &operatorv1alpha1.ClusterConnection{}
	err := r.Get(ctx, req.NamespacedName, clusterConnection)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the Secret
	secretName := clusterConnection.Spec.SecretRef.Name
	secretNamespace := clusterConnection.Spec.SecretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = clusterConnection.Namespace
	}

	secret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Error(err, "Secret not found", "Secret.Name", secretName, "Secret.Namespace", secretNamespace)
			r.updateStatus(ctx, clusterConnection, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionFalse, operatorv1alpha1.ReasonSecretNotFound, fmt.Sprintf("Secret %s/%s not found", secretNamespace, secretName))
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		log.Error(err, "Failed to fetch Secret", "Secret.Name", secretName, "Secret.Namespace", secretNamespace)
		return ctrl.Result{}, err
	}

	log.Info("Successfully fetched secret", "Secret.Name", secret.Name, "Secret.Namespace", secret.Namespace)

	// Build REST config from secret
	config, err := r.buildRESTConfig(clusterConnection.Spec.URL, secret)
	if err != nil {
		log.Error(err, "Failed to build REST config")
		r.updateStatus(ctx, clusterConnection, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionFalse, operatorv1alpha1.ReasonInvalidSecret, err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Check reachability
	err = r.checkReachability(config)
	if err != nil {
		log.Error(err, "Cluster unreachable")
		r.updateStatus(ctx, clusterConnection, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, operatorv1alpha1.ReasonConnectionFailed, err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	log.Info("Cluster is reachable")
	r.updateStatus(ctx, clusterConnection, operatorv1alpha1.ConditionTypeConnected, metav1.ConditionTrue, operatorv1alpha1.ReasonConnectionSuccess, "Successfully connected to remote cluster")

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ClusterConnectionReconciler) buildRESTConfig(url string, secret *corev1.Secret) (*rest.Config, error) {
	config := &rest.Config{
		Host: url,
	}

	// Basic Auth
	if username, ok := secret.Data["username"]; ok {
		config.Username = string(username)
	}
	if password, ok := secret.Data["password"]; ok {
		config.Password = string(password)
	}

	// Bearer Token
	if token, ok := secret.Data["bearerToken"]; ok {
		config.BearerToken = string(token)
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

func (r *ClusterConnectionReconciler) checkReachability(config *rest.Config) error {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return err
	}

	// discoveryClient doesn't support context in ServerVersion yet (in some versions)
	// but we can check if it has a RESTClient with context support if needed.
	// Actually, DiscoveryClient uses a RESTClient.

	_, err = discoveryClient.ServerVersion()
	if err != nil {
		return err
	}

	return nil
}

func (r *ClusterConnectionReconciler) updateStatus(ctx context.Context, clusterConnection *operatorv1alpha1.ClusterConnection, condType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: clusterConnection.Generation,
	}

	// Ensure opposite conditions are updated too
	if condType == operatorv1alpha1.ConditionTypeConnected && status == metav1.ConditionTrue {
		r.setCondition(&clusterConnection.Status.Conditions, condition)
		r.setCondition(&clusterConnection.Status.Conditions, metav1.Condition{
			Type:               operatorv1alpha1.ConditionTypeUnreachable,
			Status:             metav1.ConditionFalse,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: clusterConnection.Generation,
		})
	} else if condType == operatorv1alpha1.ConditionTypeUnreachable && status == metav1.ConditionTrue {
		r.setCondition(&clusterConnection.Status.Conditions, condition)
		r.setCondition(&clusterConnection.Status.Conditions, metav1.Condition{
			Type:               operatorv1alpha1.ConditionTypeConnected,
			Status:             metav1.ConditionFalse,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: clusterConnection.Generation,
		})
	} else {
		r.setCondition(&clusterConnection.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, clusterConnection); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update ClusterConnection status")
	}

	// Propagate to Cluster CRD
	r.propagateToCluster(ctx, clusterConnection, condition)
}

func (r *ClusterConnectionReconciler) setCondition(conditions *[]metav1.Condition, newCondition metav1.Condition) {
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

func (r *ClusterConnectionReconciler) propagateToCluster(ctx context.Context, clusterConnection *operatorv1alpha1.ClusterConnection, condition metav1.Condition) {
	log := logf.FromContext(ctx)

	// Find the Cluster associated with this ClusterConnection.
	// We assume they have the same name or are linked via labels.
	// For now, let's look for a Cluster with the same name.
	cluster := &operatorv1alpha1.Cluster{}
	err := r.Get(ctx, types.NamespacedName{Name: clusterConnection.Name}, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			// Try to find by label if same name doesn't work?
			// Or maybe the user didn't specify.
			return
		}
		log.Error(err, "Failed to find associated Cluster", "Cluster.Name", clusterConnection.Name)
		return
	}

	r.setCondition(&cluster.Status.Conditions, condition)

	// Update Cluster status
	if err := r.Status().Update(ctx, cluster); err != nil {
		log.Error(err, "Failed to update Cluster status", "Cluster.Name", cluster.Name)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.ClusterConnection{}).
		Named("clusterconnection").
		Complete(r)
}
