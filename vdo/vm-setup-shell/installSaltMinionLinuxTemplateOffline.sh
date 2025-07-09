#!/usr/bin/env bash
set -euo pipefail

# Trap errors and report line number
trap 'echo "[ERROR] A fatal error occurred at line $LINENO. Exiting." >&2' ERR

# Logging helpers
log_info() {
    echo "[INFO] $*"
}
log_error() {
    echo "[ERROR] $*" >&2
}

# Define download folder
DOWNLOAD_FOLDER="/tmp/ArkloudDownloads"
if [ ! -d "$DOWNLOAD_FOLDER" ]; then
    log_info "Creating download folder at $DOWNLOAD_FOLDER"
    mkdir -p "$DOWNLOAD_FOLDER"
fi

# Detect package manager (we're focusing on Debian-based systems)
log_info "Detecting package manager..."
IS_RHEL=false
IS_DEBIAN=false
if command -v yum >/dev/null 2>&1; then
    log_info "Detected RHEL-based system (yum)."
    IS_RHEL=true
elif command -v apt-get >/dev/null 2>&1; then
    log_info "Detected Debian-based system (apt-get)."
    IS_DEBIAN=true
else
    log_error "No recognized package manager found (yum or apt-get). Exiting."
    exit 1
fi

# For Debian systems, download and install the required .deb packages in order.
if [ "$IS_DEBIAN" = true ]; then
    log_info "Installing DEB packages..."

    # Define a helper function to download and install a .deb file.
    fetch_and_install_deb() {
        local url="$1"
        local outfile="$2"
        if [ -z "$url" ]; then
            log_error "No URL provided for $outfile, skipping."
            return 1
        fi
        log_info "Downloading from $url"
        if ! curl -fSL "$url" -o "$outfile"; then
            log_error "Failed to download from $url"
            exit 1
        fi
        log_info "Installing $outfile"
        if ! dpkg -i "$outfile"; then
            log_info "Attempting to fix dependencies for $outfile..."
            apt-get update -y
            apt-get install -f -y
        fi
    }

    # Install dependencies in order.
    fetch_and_install_deb "$AZURE_NCAL_DEB_URL"         "$DOWNLOAD_FOLDER/ncal.deb"
    fetch_and_install_deb "$AZURE_BSDMAINUTILS_DEB_URL" "$DOWNLOAD_FOLDER/bsdmainutils.deb"
    fetch_and_install_deb "$AZURE_DCTRL_TOOLS_DEB_URL"  "$DOWNLOAD_FOLDER/dctrl-tools.deb"
    fetch_and_install_deb "$AZURE_SALT_COMMON_DEB_URL"  "$DOWNLOAD_FOLDER/salt-common.deb"
    fetch_and_install_deb "$AZURE_SALT_MINION_DEB_URL"  "$DOWNLOAD_FOLDER/salt-minion.deb"
fi

# Update the minion configuration
if [ -f /etc/salt/minion ]; then
    log_info "Updating /etc/salt/minion configuration with master: $SALT_MASTER"
    # Remove any existing master configuration (even commented lines)
    sed -i '/^[[:space:]]*master:/d' /etc/salt/minion
    # Append the correct master setting
    echo "master: $SALT_MASTER" >> /etc/salt/minion
else
    log_info "/etc/salt/minion not found. Assuming the package creates it on first run."
fi

# Restart the salt-minion service to apply the new configuration
log_info "Restarting salt-minion service..."
if command -v systemctl >/dev/null 2>&1; then
    systemctl restart salt-minion || log_error "Failed to restart salt-minion service via systemctl."
else
    service salt-minion restart || log_error "Failed to restart salt-minion service via SysV."
fi

log_info "Salt installation and startup completed successfully!"
