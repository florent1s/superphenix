#!/bin/bash
set -euo pipefail

# The operator reconciles a single aggregated kubeconfig Secret
# (superphenix-toolbox-kubeconfig) that is mounted at /etc/superphenix/toolbox/kubeconfig.
# Kubernetes updates the mounted file in place whenever the Secret changes,
# so the merged kubeconfig is always up to date without any extra work.
KUBECONFIG_FILE="${KUBECONFIG_FILE:-/etc/superphenix/toolbox/kubeconfig}"
KUBE_DIR="${HOME}/.kube"

mkdir -p "${KUBE_DIR}"

link_kubeconfig() {
    if [[ -f "${KUBECONFIG_FILE}" ]]; then
        # Symlink the operator-managed kubeconfig into ~/.kube/config so that
        # all tools (kubectl, helm, k9s) pick it up without extra configuration.
        ln -sf "${KUBECONFIG_FILE}" "${KUBE_DIR}/config"
        echo "[toolbox] Kubeconfig linked: ${KUBECONFIG_FILE} -> ${KUBE_DIR}/config"
    else
        echo "[toolbox] Kubeconfig not yet available at ${KUBECONFIG_FILE}, waiting..."
    fi
}

echo "[toolbox] Superphenix toolbox starting."
echo "[toolbox] Kubeconfig source: ${KUBECONFIG_FILE}"
echo "[toolbox] The operator keeps the kubeconfig up to date as cluster credentials change."

link_kubeconfig

# Keep the container alive. The kubelet updates the mounted Secret file in
# place when credentials are rotated, so no polling loop is needed.
exec sleep infinity
