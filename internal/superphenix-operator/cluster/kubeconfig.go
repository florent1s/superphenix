package cluster

import (
	"context"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

const (
	// ToolboxKubeconfigSecretName is the name of the aggregated kubeconfig Secret mounted by the toolbox.
	ToolboxKubeconfigSecretName = "superphenix-toolbox-kubeconfig"
)

// reconcileToolboxKubeconfigSecret rebuilds the aggregated kubeconfig Secret that the toolbox
// sidecar mounts. It iterates over all remote Cluster CRs in the operator namespace, builds a
// kubeconfig entry for each one, and writes the merged result into a single Secret.
// The Secret is updated on every reconcile so credential rotations are picked up automatically.
func (r *Reconciler) reconcileToolboxKubeconfigSecret(ctx context.Context) error {
	log := logf.FromContext(ctx)

	clusterList := &operatorv1alpha1.ClusterList{}
	if err := r.List(ctx, clusterList, client.InNamespace(r.OperatorNamespace)); err != nil {
		return fmt.Errorf("failed to list clusters: %w", err)
	}

	merged := clientcmdapi.NewConfig()

	for i := range clusterList.Items {
		cluster := &clusterList.Items[i]

		if cluster.Spec.Connection == nil {
			continue
		}

		switch cluster.Spec.Connection.Mode {
		case operatorv1alpha1.ConnectionModeLocal:
			if err := mergeLocalClusterIntoConfig(merged, cluster.Name); err != nil {
				log.Error(err, "Failed to build kubeconfig entry for local cluster, skipping", "cluster", cluster.Name)
			}

		case operatorv1alpha1.ConnectionModeRemote:
			if cluster.Spec.Connection.SecretRef == nil {
				continue
			}

			connSecretNamespace := cluster.Spec.Connection.SecretRef.Namespace
			if connSecretNamespace == "" {
				connSecretNamespace = cluster.Namespace
			}

			connSecret := &corev1.Secret{}
			if err := r.Get(ctx, types.NamespacedName{
				Name:      cluster.Spec.Connection.SecretRef.Name,
				Namespace: connSecretNamespace,
			}, connSecret); err != nil {
				log.Error(err, "Failed to fetch connection secret for cluster, skipping", "cluster", cluster.Name)
				continue
			}

			connData := r.extractConnectionData(connSecret)
			if err := mergeClusterIntoConfig(merged, cluster.Name, cluster.Spec.Connection.URL, connData); err != nil {
				log.Error(err, "Failed to build kubeconfig entry for cluster, skipping", "cluster", cluster.Name)
				continue
			}
		}
	}

	kubeconfigBytes, err := clientcmd.Write(*merged)
	if err != nil {
		return fmt.Errorf("failed to serialize merged kubeconfig: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ToolboxKubeconfigSecretName,
			Namespace: r.OperatorNamespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		secret.Data = map[string][]byte{
			"kubeconfig": kubeconfigBytes,
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to upsert toolbox kubeconfig secret: %w", err)
	}

	log.Info("Successfully reconciled toolbox kubeconfig secret", "secret", ToolboxKubeconfigSecretName)
	return nil
}

// mergeLocalClusterIntoConfig adds a kubeconfig entry for the local cluster using the in-cluster REST config.
func mergeLocalClusterIntoConfig(cfg *clientcmdapi.Config, clusterName string) error {
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get in-cluster REST config: %w", err)
	}

	caData := restConfig.TLSClientConfig.CAData
	if len(caData) == 0 && restConfig.TLSClientConfig.CAFile != "" {
		caData, err = os.ReadFile(restConfig.TLSClientConfig.CAFile)
		if err != nil {
			return fmt.Errorf("failed to read CA file %s: %w", restConfig.TLSClientConfig.CAFile, err)
		}
	}

	certData := restConfig.TLSClientConfig.CertData
	if len(certData) == 0 && restConfig.TLSClientConfig.CertFile != "" {
		certData, err = os.ReadFile(restConfig.TLSClientConfig.CertFile)
		if err != nil {
			return fmt.Errorf("failed to read cert file %s: %w", restConfig.TLSClientConfig.CertFile, err)
		}
	}

	keyData := restConfig.TLSClientConfig.KeyData
	if len(keyData) == 0 && restConfig.TLSClientConfig.KeyFile != "" {
		keyData, err = os.ReadFile(restConfig.TLSClientConfig.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to read key file %s: %w", restConfig.TLSClientConfig.KeyFile, err)
		}
	}

	token := restConfig.BearerToken
	if token == "" && restConfig.BearerTokenFile != "" {
		tokenBytes, err := os.ReadFile(restConfig.BearerTokenFile)
		if err != nil {
			return fmt.Errorf("failed to read bearer token file %s: %w", restConfig.BearerTokenFile, err)
		}
		token = strings.TrimSpace(string(tokenBytes))
	}

	conn := &connectionData{
		bearerToken: token,
		caData:      caData,
		certData:    certData,
		keyData:     keyData,
		insecure:    restConfig.TLSClientConfig.Insecure,
		serverName:  restConfig.TLSClientConfig.ServerName,
		username:    restConfig.Username,
		password:    restConfig.Password,
	}
	return mergeClusterIntoConfig(cfg, clusterName, restConfig.Host, conn)
}

// mergeClusterIntoConfig adds a cluster, user, and context entry into the given Config.
func mergeClusterIntoConfig(cfg *clientcmdapi.Config, clusterName, serverURL string, conn *connectionData) error {
	authInfo := clientcmdapi.NewAuthInfo()
	if conn.bearerToken != "" {
		authInfo.Token = conn.bearerToken
	}
	if conn.username != "" {
		authInfo.Username = conn.username
	}
	if conn.password != "" {
		authInfo.Password = conn.password
	}
	if len(conn.certData) > 0 {
		authInfo.ClientCertificateData = conn.certData
	}
	if len(conn.keyData) > 0 {
		authInfo.ClientKeyData = conn.keyData
	}

	clusterEntry := clientcmdapi.NewCluster()
	clusterEntry.Server = serverURL
	if len(conn.caData) > 0 {
		clusterEntry.CertificateAuthorityData = conn.caData
	}
	clusterEntry.InsecureSkipTLSVerify = conn.insecure
	if conn.serverName != "" {
		clusterEntry.TLSServerName = conn.serverName
	}

	cfg.Clusters[clusterName] = clusterEntry
	cfg.AuthInfos[clusterName] = authInfo
	cfg.Contexts[clusterName] = &clientcmdapi.Context{
		Cluster:  clusterName,
		AuthInfo: clusterName,
	}

	// Set the first cluster as the default context if none is set yet.
	if cfg.CurrentContext == "" {
		cfg.CurrentContext = clusterName
	}

	return nil
}
