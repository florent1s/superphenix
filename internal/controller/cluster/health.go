package cluster

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

// reconcileHealth checks the connectivity of the cluster (local or remote) and updates its status.
// If the cluster is unreachable, it returns a Requeue result.
func (r *Reconciler) reconcileHealth(ctx context.Context, cluster *operatorv1alpha1.Cluster) (string, ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var config *rest.Config
	var err error

	config, err = r.getRESTConfigForCluster(ctx, cluster)
	if err != nil {
		log.Error(err, "Failed to build REST config for cluster")
		// The status condition is already set inside getRESTConfigForCluster
		return "", ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	version, err := r.checkReachability(config)
	if err != nil {
		log.Error(err, "Cluster unreachable")
		r.updateStatusWithPhase(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, operatorv1alpha1.ReasonConnectionFailed, err.Error(), "Error")
		return "", ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	log.Info("Cluster is reachable", "version", version)
	r.updateStatus(ctx, cluster, operatorv1alpha1.ConditionTypeConnected, metav1.ConditionTrue, operatorv1alpha1.ReasonConnectionSuccess, "Successfully connected to cluster")

	return version, ctrl.Result{}, nil
}

// getRESTConfigForCluster generates the configuration to connect to a Kubernetes cluster
func (r *Reconciler) getRESTConfigForCluster(ctx context.Context, cluster *operatorv1alpha1.Cluster) (*rest.Config, error) {
	log := logf.FromContext(ctx)

	if cluster.Spec.Connection == nil {
		err := fmt.Errorf("%w: connection configuration is missing", errConfig)
		r.updateStatusWithPhase(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, operatorv1alpha1.ReasonConnectionConfigError, err.Error(), "Error")
		return nil, err
	}

	if cluster.Spec.Connection.Mode == operatorv1alpha1.ConnectionModeLocal {
		log.Info("Using local connection mode")
		config, err := ctrl.GetConfig()
		if err != nil {
			r.updateStatusWithPhase(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, operatorv1alpha1.ReasonConnectionFailed, err.Error(), "Error")
			return nil, err
		}
		return config, nil
	}

	// For remote mode, we need URL and SecretRef
	if cluster.Spec.Connection.URL == "" {
		err := fmt.Errorf("%w: connection URL must be provided in Remote mode", errConfig)
		r.updateStatusWithPhase(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, operatorv1alpha1.ReasonConnectionConfigError, err.Error(), "Error")
		return nil, err
	}
	if cluster.Spec.Connection.SecretRef == nil {
		err := fmt.Errorf("%w: secret reference must be provided in Remote mode", errConfig)
		r.updateStatusWithPhase(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, operatorv1alpha1.ReasonConnectionConfigError, err.Error(), "Error")
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
		r.updateStatusWithPhase(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, reason, err.Error(), "Error")
		return nil, err
	}

	log.Info("Successfully fetched secret", "Secret.Name", secret.Name, "Secret.Namespace", secret.Namespace)

	// Build REST config from secret
	config, err := r.buildRESTConfig(cluster.Spec.Connection.URL, secret)
	if err != nil {
		reason := operatorv1alpha1.ReasonConnectionFailed
		if errors.Is(err, errInvalidSecret) {
			reason = operatorv1alpha1.ReasonInvalidSecret
		}
		r.updateStatusWithPhase(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, reason, err.Error(), "Error")
		return nil, err
	}
	return config, nil
}

func (r *Reconciler) buildRESTConfig(url string, secret *corev1.Secret) (*rest.Config, error) {
	config := &rest.Config{
		Host: url,
	}

	connData := r.extractConnectionData(secret)

	if !connData.hasAuth {
		return nil, fmt.Errorf("%w: secret must contain authentication credentials (username/password, bearerToken or certData/keyData)", errInvalidSecret)
	}

	config.Username = connData.username
	config.Password = connData.password
	config.BearerToken = connData.bearerToken

	// TLS Config
	config.TLSClientConfig = rest.TLSClientConfig{
		CertData:   connData.certData,
		KeyData:    connData.keyData,
		Insecure:   connData.insecure,
		ServerName: connData.serverName,
	}

	if !connData.insecure {
		config.TLSClientConfig.CAData = connData.caData
	}

	return config, nil
}

// checkReachability tries to connect to the cluster's discovery API to check if it's alive.
func (r *Reconciler) checkReachability(config *rest.Config) (string, error) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return "", err
	}

	version, err := discoveryClient.ServerVersion()
	if err != nil {
		return "", err
	}
	return version.GitVersion, nil
}
