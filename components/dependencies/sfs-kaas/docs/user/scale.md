# Cluster scaling

## Horizontal scaling

To add nodes to your cluster, go to your projects repository and proceed as follows:
1. Open `clusters/values.yaml`.
1. Increase `.kubernetes.<cluster-name>.workers.instances.<worker-pool>.deployment.replicas`.
1. Commit and push the changes.
1. Go to the your project's page on the selfservice ArgoCD and synchronize the changes.

Alternatively you can also add a new worker pool by adding its specification under `.kubernetes.<cluster-name>.workers.instances` like in this minimal example:
```yaml
        pool-1:
          deployment:
            replicas: 2
          template:
            version: 1
            cores: 2
            memory: 6Gi
            interfaces:
              - subnet: "<my-subnet>"
            bootDisk:
              storageClassName: default
```

## Vertical scaling

To increase the size of exiting nodes, go to your projects repository and proceed as follows:
1. Open `clusters/values.yaml`.
1. Update `.kubernetes.<cluster-name>.workers.instances.<worker-pool>.template.cores`, `.memory` or `.bootDisk.storage` as desired.
1. Increment `.kubernetes.<cluster-name>.workers.instances.<worker-pool>.template.version`.  
  This is required for the specification update to take effect.
1. Commit and push the changes.
1. Go to the your project's page on the selfservice ArgoCD and synchronize the changes.

These steps will trigger a rolling update of the worker VMs which will replace them one by one by default.  
You can set `.kubernetes.<cluster-name>.workers.instances.<worker-pool>.template.maxSurge` and `.maxUnavailable` to control the rolling update.
