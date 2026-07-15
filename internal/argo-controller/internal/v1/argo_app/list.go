package argoApp

import (
	"argo-controller/internal/v1/models/view"
	"argo-controller/pkg/config"
	"context"
	"fmt"
	logger "utils/log"

	spxId "superphenix-id"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ListApps(ctx context.Context, namespace string) []view.AppView {
	log := logger.GetLogger(ctx)
	projectLabel := fmt.Sprintf("%s=%s", spxId.SpxLabelProjectID, namespace)
	app, err := config.ArgoClient.Applications(namespace).List(ctx, v1.ListOptions{
		LabelSelector: projectLabel,
	})
	if err != nil {
		log.Err(err).Str("namespace", namespace).Msg("Error listing Argo Apps")
		return make([]view.AppView, 0)
	}
	return view.AppsToView(app.Items)
}
