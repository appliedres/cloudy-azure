# ── Stage 3 : RDP Setup ─────────────────────────────────────────────────────
log "Installing and configuring RDP server..."

# Detect distro
if [ -f /etc/os-release ]; then
  . /etc/os-release
else
  log "[ERROR] Cannot find /etc/os-release; aborting."
  exit 1
fi

case "$ID" in
  debian|ubuntu)
    log "Installing xrdp and XFCE via apt for $ID $VERSION_ID"
    sudo apt-get update -y
    sudo apt-get install -y xrdp xfce4 xfce4-terminal
    ;;

  rhel|centos|rocky|almalinux|fedora)
    log "Installing xrdp and XFCE for $ID $VERSION_ID"

    RHEL_VERSION=$(rpm -E %rhel)
    if [[ -z "$RHEL_VERSION" ]]; then
      log "[ERROR] Could not determine RHEL version for EPEL install."
      exit 1
    fi

    # Try standard EPEL install first
    if ! sudo dnf install -y epel-release; then
      log "epel-release install failed, falling back to direct EPEL RPM"

      if ! command -v curl >/dev/null 2>&1; then
        log "curl not found, installing it first..."
        sudo dnf install -y curl || { log "[ERROR] Failed to install curl"; exit 1; }
      fi

      EPEL_URL="https://dl.fedoraproject.org/pub/epel/epel-release-latest-${RHEL_VERSION}.noarch.rpm"
      log "Checking EPEL URL: $EPEL_URL"

      if curl -Is "$EPEL_URL" | head -n 1 | grep -q "200"; then
        if ! sudo dnf install -y "$EPEL_URL" --nogpgcheck; then
          log "[ERROR] Failed to install EPEL from $EPEL_URL"
          exit 1
        fi
      else
        log "[ERROR] EPEL URL not reachable or invalid: $EPEL_URL"
        exit 1
      fi
    fi

    # Enable XRDP Copr repository if needed other installs failed
    if ! dnf list xrdp &>/dev/null; then
      log "xrdp not found in current repos, enabling Copr repo..."
      sudo dnf install -y 'dnf-command(copr)'
      sudo dnf copr enable -y @xrdp/xrdp
    fi

    # Install full XFCE and xorgxrdp
    sudo dnf groupinstall -y "Xfce"
    sudo dnf install -y xrdp xorgxrdp --setopt=install_weak_deps=False
    ;;

  *)
    log "[ERROR] Unsupported distribution: $ID for RDP setup."
    exit 1
    ;;
esac

# Enable and start xrdp
log "Enabling and starting xrdp service"
sudo systemctl enable xrdp
sudo systemctl start xrdp

# Enable automatic home directory creation
log "Configuring pam_mkhomedir for session login"
if ! grep -q pam_mkhomedir.so /etc/pam.d/common-session; then
  echo "session required pam_mkhomedir.so skel=/etc/skel umask=0077" | sudo tee -a /etc/pam.d/common-session
fi

if systemctl is-enabled --quiet oddjobd 2>/dev/null || systemctl list-unit-files | grep -q oddjobd; then
  log "Enabling and starting oddjobd for mkhomedir support"
  sudo systemctl enable oddjobd
  sudo systemctl start oddjobd
fi

# Create default .Xclients in skeleton for future users
echo "startxfce4" | sudo tee /etc/skel/.Xclients
chmod +x /etc/skel/.Xclients

# If the domain user is resolvable, create home and session file explicitly
if id "$AD_USER" &>/dev/null; then
  USER_UID=$(id -u "$AD_USER")
  USER_GID=$(id -g "$AD_USER")
  HOME_DIR="/home/$AD_USER"
  mkdir -p "$HOME_DIR"
  echo "startxfce4" > "$HOME_DIR/.Xclients"
  chown "$USER_UID:$USER_GID" "$HOME_DIR" "$HOME_DIR/.Xclients"
  chmod +x "$HOME_DIR/.Xclients"
fi

# Open RDP port
if command -v ufw >/dev/null 2>&1; then
  log "Allowing RDP through ufw firewall (port 3389)"
  sudo ufw allow 3389/tcp
elif command -v firewall-cmd >/dev/null 2>&1; then
  log "Allowing RDP through firewalld (port 3389)"
  sudo firewall-cmd --permanent --add-port=3389/tcp
  sudo firewall-cmd --reload
else
  log "[WARNING] No known firewall tool detected. Skipping firewall configuration."
fi

# Allow anyone to start X sessions
sudo sed -i 's/^allowed_users=.*/allowed_users=anybody/' /etc/X11/Xwrapper.config || true

# Configure login banner
HOSTNAME=$(hostname -s)

log "Setting login banner in /etc/issue"

cat <<'EOF' | tee /etc/issue >/dev/null
────────────────────────────────────────────────────────────
Welcome to VM: $HOSTNAME

To log in via RDP or SSH, use your domain credentials:

Format:
    john.doe@yourdomain.onmicrosoft.us

Notes:
  - Password is your Entra login password.
  - Access is restricted to approved users.

────────────────────────────────────────────────────────────
EOF

# Restart XRDP to apply all changes
sudo systemctl restart xrdp

# Confirm service is running
if systemctl is-active --quiet xrdp; then
  log "RDP service (xrdp) is active and running"
else
  log "[ERROR] RDP service (xrdp) is not running properly."
  exit 1
fi

log "Stage 3 – RDP setup complete."
