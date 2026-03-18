package cluster

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
	secretName := fmt.Sprintf("argocd-secret-%s", cluster.Name)
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

	decode := func(b []byte) []byte {
		s := strings.TrimSpace(string(b))
		if s == "true" || s == "false" {
			return []byte(s)
		}
		decoded, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return b
		}
		return decoded
	}

	// Helper to get data from either Data or StringData
	getData := func(key string) []byte {
		if val, ok := secret.Data[key]; ok {
			return val
		}
		if val, ok := secret.StringData[key]; ok {
			return []byte(val)
		}
		return nil
	}

	// Auth
	if token := getData("bearerToken"); token != nil {
		data.bearerToken = string(decode(token))
		data.hasAuth = true
	}
	if username := getData("username"); username != nil {
		data.username = string(decode(username))
		data.hasAuth = true
	}
	if password := getData("password"); password != nil {
		data.password = string(decode(password))
		data.hasAuth = true
	}

	// TLS
	if caData := getData("caData"); caData != nil {
		data.caData = decode(caData)
		if len(data.caData) > 0 {
			data.tlsClientConfig["caData"] = base64.StdEncoding.EncodeToString(data.caData)
		}
	}
	if certData := getData("certData"); certData != nil {
		data.certData = decode(certData)
		if len(data.certData) > 0 {
			data.tlsClientConfig["certData"] = base64.StdEncoding.EncodeToString(data.certData)
		}
		if len(data.certData) > 0 && len(getData("keyData")) > 0 {
			data.hasAuth = true
		}
	}
	if keyData := getData("keyData"); keyData != nil {
		data.keyData = decode(keyData)
		if len(data.keyData) > 0 {
			data.tlsClientConfig["keyData"] = base64.StdEncoding.EncodeToString(data.keyData)
		}
	}
	if insecure := getData("insecure"); insecure != nil {
		data.insecure = string(decode(insecure)) == "true"
		data.tlsClientConfig["insecure"] = data.insecure
	}
	if serverName := getData("serverName"); serverName != nil {
		data.serverName = string(decode(serverName))
		data.tlsClientConfig["serverName"] = data.serverName
	}

	return data
}
