package loadBalancer

import (
	"context"
	"superphenix-controller/internal/informers"
	"superphenix-controller/internal/models/view"

	spxId "superphenix-id"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func ListLoadBalancer(ctx context.Context, namespace string) []view.LBView {
	list := informers.WatcherSet[informers.SwitchLBRules].List()
	lbs := make([]view.LBView, 0)
	for _, item := range list {
		lb := item.(*unstructured.Unstructured)
		if lb.GetLabels()[spxId.SpxLabelProjectID] == namespace {
			lbView := view.UnstructuredLBToView(lb)
			lbs = append(lbs, lbView)
		}
	}

	return lbs
}
