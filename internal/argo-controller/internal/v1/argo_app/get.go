package argoApp

import (
	"argo-controller/internal/v1/models/view"
	"argo-controller/pkg/config"
	"context"
	logger "utils/log"

	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetArgoApp(ctx context.Context, name, namespace string) (view.AppView, error) {
	log := logger.GetLogger(ctx)

	get, err := config.ArgoClient.Applications(namespace).Get(ctx, name, k8smetav1.GetOptions{})
	if err != nil {
		log.Err(err).Str("name", name).Msg("Error getting argo app")
	}
	return view.AppToView(*get), err
}
