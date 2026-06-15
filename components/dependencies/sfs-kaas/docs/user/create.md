# Cluster creation

## Prerequisites

SNAT needs to be configured in the cluster's subnet or the nodes won't be able to talk to their controlplane.

## Deploy a cluster

To create a Kubernetes cluster using GitOps, go to your projects repository and proceed as follows:
1. Create or open `clusters/values.yaml`
1. Under the `.kubernetes` key, add the specification of the desired cluster(s). A minimal example is provided below,
    see [sfs-kaas/values.yaml](/values.yaml) for the complete overview of configuration options.
    ```yaml
      my-cluster:
        name: "my-cluster"
        location: "aq01-test01"
        kubeVersion: v1.33.4
        workers:
          instances:
            small-1:
              deployment:
                replicas: 2
              template:
                version: 1
                cores: 2
                memory: 4Gi
                interfaces:
                  - subnet: "<my-subnet>"
                bootDisk:
                  storageClassName: default
        kaasEssentials:
          storageClasses:
            abc:
              isDefaultClass: true
              tenantClass: abc
              infraClass: default
          snapshotClasses:
            abc:
              tenantClass: abc
              infraClass: default
    ```
1. Commit and push the changes.
1. Go to the your project's page on the selfservice ArgoCD and synchronize the changes.
1. Once all the resources are created, you can find the `kubeconfig` file as a secret named `<clusterID>-kubeconfig` in your project's namespace.
