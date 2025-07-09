# ── Stage 2 : Salt minion install ────────────────────────────────────────────
SALT_MASTER="$SALT_MASTER"


# ── Detect distro ──────────────────────────────────────────────────────────
if [ -f /etc/os-release ]; then
  . /etc/os-release
else
  echo "[ERROR] Cannot find /etc/os-release; aborting." >&2
  exit 1
fi

case "$ID" in
  debian|ubuntu)
    log "Installing Salt Minion via APT for $ID $VERSION_ID"

	# Ensure keyrings dir exists
    sudo mkdir -p /etc/apt/keyrings

	# Download public key
	sudo curl -fsSL https://packages.broadcom.com/artifactory/api/security/keypair/SaltProjectKey/public | sudo tee /etc/apt/keyrings/salt-archive-keyring.pgp
	
	# Create apt repo target configuration
	sudo curl -fsSL https://github.com/saltstack/salt-install-guide/releases/latest/download/salt.sources | sudo tee /etc/apt/sources.list.d/salt.sources

    sudo apt-get update -y
    export DEBIAN_FRONTEND=noninteractive
    sudo apt-get install -y salt-minion=3006.10 salt-common=3006.10
    ;;

  rhel|centos|rocky|almalinux|fedora)
    log "Installing Salt Minion via YUM/DNF for $ID $VERSION_ID"

    sudo curl -fsSL https://github.com/saltstack/salt-install-guide/releases/latest/download/salt.repo \
      -o /etc/yum.repos.d/salt.repo

    sudo dnf clean expire-cache
    sudo dnf install -y salt-minion-3006.10
    ;;

  *)
    echo "[ERROR] Unsupported distribution: $ID" >&2
    exit 1
    ;;
esac

# Update the minion configuration
if [ -f /etc/salt/minion ]; then
    log "Updating /etc/salt/minion configuration with master: $SALT_MASTER"
    sed -i '/^[[:space:]]*master:/d' /etc/salt/minion
    echo "master: $SALT_MASTER" | sudo tee -a /etc/salt/minion
else
    log "/etc/salt/minion not found. Assuming the package creates it on first run."
fi

# Restart the salt-minion service
log "Restarting salt-minion service..."
if command -v systemctl >/dev/null 2>&1; then
    sudo systemctl restart salt-minion || log "[ERROR] Failed to restart salt-minion service via systemctl."
else
    sudo service salt-minion restart || log "[ERROR] Failed to restart salt-minion service via SysV."
fi

# ── Wait for service to stabilize ──────────────────────────────────────────
sleep 5

# ── Check minion logs for master connection ────────────────────────────────
if sudo grep -qi "Authentication with master at" /var/log/salt/minion; then
  log "Salt Minion attempted to connect to master $SALT_MASTER"
else
  log "[WARNING] Salt Minion log does not yet show a connection to the master."
fi

# ── Optionally check ping locally (does NOT verify master, but confirms minion works) ──
if sudo salt-call --local test.ping; then
  log "Salt Minion installed and working (local test.ping passed)"
else
  log "[WARNING] salt-call test.ping failed (local). Check service status and config."
fi
