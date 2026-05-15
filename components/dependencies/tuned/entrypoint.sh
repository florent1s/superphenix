#!/bin/sh
set -e

PROFILES_CM_DIR="/etc/tuned/profiles-cm"
TUNED_DIR="/etc/tuned"
DEFAULT_PROFILE="${DEFAULT_PROFILE:-throughput-performance}"
PROFILE_LABEL_KEY="${PROFILE_LABEL_KEY:-performance.superphenix.net/profile}"
NODE_NAME="${NODE_NAME:-}"

# Install custom profiles from the ConfigMap mount
if [ -d "$PROFILES_CM_DIR" ]; then
  echo "[!] Installing custom profiles from ConfigMap..."
  for profile_file in "$PROFILES_CM_DIR"/*; do
    profile_name=$(basename "$profile_file")
    mkdir -p "${TUNED_DIR}/${profile_name}"
    cp "$profile_file" "${TUNED_DIR}/${profile_name}/tuned.conf"
    echo "[+] Installed profile: ${profile_name}"
  done
fi

# Resolve which profile to apply
PROFILE="$DEFAULT_PROFILE"

if [ -n "$NODE_NAME" ]; then
  TOKEN_FILE="/var/run/secrets/kubernetes.io/serviceaccount/token"
  CA_CERT="/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
  API_SERVER="https://kubernetes.default.svc"

  if [ -f "$TOKEN_FILE" ] && [ -f "$CA_CERT" ]; then
    LABEL_VALUE=$(python3 - <<EOF
import urllib.request, json, ssl, sys

token = open("${TOKEN_FILE}").read().strip()
ctx = ssl.create_default_context()
ctx.load_verify_locations("${CA_CERT}")

req = urllib.request.Request(
    "${API_SERVER}/api/v1/nodes/${NODE_NAME}",
    headers={"Authorization": "Bearer " + token},
)
try:
    resp = urllib.request.urlopen(req, context=ctx)
    node = json.loads(resp.read())
    label = node.get("metadata", {}).get("labels", {}).get("${PROFILE_LABEL_KEY}", "")
    print(label)
except Exception as e:
    print("", end="")
    sys.stderr.write("[!] Warning: could not query node labels: " + str(e) + "\n")
EOF
    )

    if [ -n "$LABEL_VALUE" ]; then
      PROFILE="$LABEL_VALUE"
      echo "[~] Using profile from node label '${PROFILE_LABEL_KEY}': ${PROFILE}"
    else
      echo "[!] No profile label found on node, using default: ${DEFAULT_PROFILE}"
    fi
  else
    echo "[!] Service account credentials not found, using default profile: ${DEFAULT_PROFILE}"
  fi
else
  echo "[~] NODE_NAME not set, using default profile: ${DEFAULT_PROFILE}"
fi

# Configure tuned to run in no-daemon mode (apply profile and exit)
cat > /etc/tuned/tuned-main.conf <<EOF
[main]
# Do not run tuned as a daemon, exit once it has configured the profile
daemon = 0
# Avoid overriding the user-defined sysctl config
reapply_sysctl = 0
EOF

# Write the selected profile so tuned picks it up on startup
echo "[+] Activating profile: ${PROFILE}"
echo "$PROFILE" > /etc/tuned/active_profile

# Apply the profile (tuned exits immediately in no-daemon mode)
tuned --no-dbus --no-socket --log=-

# Keep the container alive
exec sleep infinity
