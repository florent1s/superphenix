package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

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

// reconcileArgoCDSecret ensures the secret used by ArgoCD to connect to the clusters is up to date
// with the connection configuration from the Cluster CRD.
func (r *Reconciler) reconcileArgoCDSecret(ctx context.Context, cluster *operatorv1alpha1.Cluster) error {
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
		if connData.bearerToken == "" && connData.username == "" && connData.password == "" && (len(connData.certData) == 0 || len(connData.keyData) == 0) {
			return fmt.Errorf("%w: secret must contain either a bearerToken, a username/password pair or certData/keyData", errInvalidSecret)
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
		r.updateStatusWithPhase(ctx, cluster, operatorv1alpha1.ConditionTypeUnreachable, metav1.ConditionTrue, reason, err.Error(), "Error")
		return err
	}

	log.Info("Successfully reconciled ArgoCD connection secret", "Secret.Name", secretName, "Secret.Namespace", r.OperatorNamespace)
	return nil
}

func (r *Reconciler) extractConnectionData(secret *corev1.Secret) *connectionData {
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
		if len(data.certData) > 0 && len(secret.Data["keyData"]) > 0 {
			data.hasAuth = true
		}
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
