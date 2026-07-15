package subnet

import (
	"context"
	"superphenix-controller/internal/informers"
	"superphenix-controller/internal/models/view"

	spxId "superphenix-id"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func ListSubnet(ctx context.Context, namespace string) []view.SubnetView {
	list := informers.WatcherSet[informers.Subnet].List()
	subnets := make([]view.SubnetView, 0)
	for _, item := range list {
		subnet := item.(*unstructured.Unstructured)
		isAllowedShared := isAllowedSharedSubnet(subnet.GetAnnotations(), namespace)
		if subnet.GetLabels()[spxId.SpxLabelProjectID] == namespace || isAllowedShared {
			subnetView := view.UnstructuredSubnetToView(subnet, isAllowedShared)
			subnets = append(subnets, subnetView)
		}
	}
	return subnets
}
