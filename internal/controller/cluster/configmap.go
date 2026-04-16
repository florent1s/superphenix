package cluster

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

// reconcileClustersConfigMap ensures that the cluster configuration entry exists in the shared ConfigMap.
// It creates the ConfigMap if it doesn't exist.
func (r *Reconciler) reconcileClustersConfigMap(ctx context.Context, cluster *operatorv1alpha1.Cluster) error {
	if r.ClustersConfigMapName == "" {
		return nil
	}

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: r.ClustersConfigMapName, Namespace: r.OperatorNamespace}, cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      r.ClustersConfigMapName,
					Namespace: r.OperatorNamespace,
				},
				Data: map[string]string{
					cluster.Name: "",
				},
			}
			return r.Create(ctx, cm)
		}
		return err
	}

	// Update the entry for this cluster if it's missing.
	// We keep other entries.
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}

	if _, ok := cm.Data[cluster.Name]; ok {
		// Entry already exists, nothing to do for now
		return nil
	}

	cm.Data[cluster.Name] = ""
	return r.Update(ctx, cm)
}

// removeClusterFromConfigMap removes the cluster entry from the shared ConfigMap.
func (r *Reconciler) removeClusterFromConfigMap(ctx context.Context, cluster *operatorv1alpha1.Cluster) error {
	if r.ClustersConfigMapName == "" {
		return nil
	}

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: r.ClustersConfigMapName, Namespace: r.OperatorNamespace}, cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if cm.Data == nil {
		return nil
	}

	if _, ok := cm.Data[cluster.Name]; !ok {
		return nil
	}

	delete(cm.Data, cluster.Name)
	return r.Update(ctx, cm)
}
