package kaas

const ClusterLabelKey = "cluster.x-k8s.io/cluster-name"
const ClusterAppNameLabelKey = "app.kubernetes.io/instance"
const KaaSPrefix = "kaas-"

// HelmChartLabelKey holds "<chart>-<version>" of the chart that rendered the cluster.
const HelmChartLabelKey = "helm.sh/chart"
