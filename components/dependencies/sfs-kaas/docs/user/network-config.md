# Network configuration

## Default configuration

By default a KaaS cluster installation will deploy `networkPolicies` which ensure that:
- Controlplane is joinable from everywhere
- Communication between worker nodes is possible
- Nodes can access the internet (to download images for ex.)
- (default mode only) Nodes can be joined by none-worker VMs inside and/or outside the subnet depending on the network configuration

## Customize the configuration

You can control the `networkPolicies` for the controlplane and worker nodes independently with the following values:
```yaml
my-cluster:
  controlPlane:
    network:
      defaultPolicies: default/none
  workers:
    network:
      defaultPolicies: default/strict/none
```

Disabling those `networkPolicies` allows you to define your own firewall rules to restrict traffic to and from the cluster.
These are the labels you can use to target the cluster components in your rules:
1. Controlplane
  ```yaml
  superphenix.net/resourceEffectiveID: <SPXID of your cluster>
  superphenix.net/workloadClass: kaas-tenant-api-server
  ```
2. Worker nodes
  ```yaml
  cluster.x-k8s.io/cluster-name: <SPXID of your cluster>
  ```
