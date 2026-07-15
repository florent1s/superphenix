package natGateway

import (
	"superphenix-controller/internal/informers"
	"superphenix-controller/internal/models/view"

	spxId "superphenix-id"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func ListNatGw(namespace string) []view.NatGwView {
	list := informers.WatcherSet[informers.NatGw].List()
	natGws := make([]view.NatGwView, 0)
	for _, item := range list {
		natGw := item.(*unstructured.Unstructured)
		if natGw.GetLabels()[spxId.SpxLabelProjectID] == namespace {
			natGwView := view.UnstructuredNatGwToView(natGw)
			natGws = append(natGws, natGwView)
		}
	}
	return natGws
}
