# Multus Helm Chart

Multus doesn't have an official Helm Chart.

We generate this one from [the official GitHub](https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/deployments/multus-daemonset-thick.yml)


!! WARNING

We need to use this patch (applied by hand) to avoid race conditions

https://github.com/k8snetworkplumbingwg/multus-cni/pull/1445/files


We also need to patch the deployment to support Talos

https://docs.siderolabs.com/kubernetes-guides/cni/multus