# Cluster upgrade

## Controlling the upgrade process

Here a few configuration options to control how the upgrade process behaves:
- Set `.kubernetes.<cluster-name>.workers.instances.<worker-pool>.template.maxSurge` and `.maxUnavailable` to control the VM rolling update.
- Set `.kubernetes.<cluster-name>.workers.instances.<worker-pool>.kubeVersion` to force a K8s version for those VMs that can differ from the global K8s version.  
  This can be useful to hold back the upgrade on certain workers until the upgrade is confirmed to be working for the rest of the cluster.

## Performing the upgrade

To upgrade your cluster in GitOps, go to your projects repository and proceed as follows:
1. Open `clusters/values.yaml`
1. Change `.kubernetes.<cluster-name>.kubeVersion` to the desired version.
1. Commit and push the changes.
1. Go to the your project's page on the selfservice ArgoCD and synchronize the changes.

These steps will trigger a controlplane upgrade. Once it is done the worker VMs will undergo a rolling update which
will replace them one by one by default.