package vdo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy-azure/storage"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
)

func (vdo *VirtualDesktopOrchestrator) virtualMachineSetupWindows(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "virtualMachineSetupWindows started")
	defer log.DebugContext(ctx, "virtualMachineSetupWindows finished")

	vdoConfig := vdo.config
	if vdo.avdManager != nil {
		log.InfoContext(ctx, "Initial VM setup - AVD enabled")

		hostPoolNamePtr, hostPoolToken, err := vdo.avdManager.PreRegister(ctx, vm)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "AVD Pre-Register failed")
		}

		script, err := vdo.buildVirtualMachineSetupScript(ctx, vdoConfig, hostPoolToken)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "Could not build powershell script (AVD enabled)")
		}

		err = vdo.vmManager.ExecuteRemotePowershell(ctx, vm.ID, script, 20*time.Minute, 15*time.Second)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "Could not run powershell (AVD enabled)")
		}

		vm, err = vdo.avdManager.PostRegister(ctx, vm, *hostPoolNamePtr)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "AVD Post-Register VM")
		}

	} else {
		log.InfoContext(ctx, "Initial VM setup - AVD disabled")
		script, err := vdo.buildVirtualMachineSetupScript(ctx, vdoConfig, nil)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "Could not build powershell script (AVD disabled)")
		}

		err = vdo.vmManager.ExecuteRemotePowershell(ctx, vm.ID, script, 10*time.Minute, 15*time.Second)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "Could not run powershell (AVD disabled)")

		}
	}

	log.InfoContext(ctx, "Initial Windows VM setup completed successfully")
	return vm, nil
}

func (vdo *VirtualDesktopOrchestrator) virtualMachineSetupLinux(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "virtualMachineSetupLinux started")
	defer log.DebugContext(ctx, "virtualMachineSetupLinux finished")

	// TODO: improve / remove online vs offline decision
	shellScript, err := vdo.buildVirtualMachineSetupScriptLinuxOnline(ctx, vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "Could not build Linux setup script")
	}

	err = vdo.vmManager.ExecuteRemoteShellScript(ctx, vm.ID, &shellScript, 10*time.Minute, 15*time.Second)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "Could not run Linux setup script")
	}

	log.InfoContext(ctx, "Initial Linux VM setup completed successfully")
	return vm, nil
}

func (vdo *VirtualDesktopOrchestrator) buildVirtualMachineSetupScriptLinuxOffline(ctx context.Context, vm *models.VirtualMachine) (string, error) {
	log := logging.GetLogger(ctx)
	cfg := vdo.config
	if cfg.SaltMinionInstall == nil {
		return "", errors.New("Salt Minion install config not provided for Linux VM setup")
	}

	saltScript, err := GenerateInstallSaltMinionScriptLinuxOffline(
		ctx,
		vdo.vmManager.Credentials,
		cfg.BinaryStorage.BlobStorageAccount,
		cfg.BinaryStorage.BlobContainer,
		cfg.SaltMinionInstall,
	)
	if err != nil {
		return "", logging.LogAndWrapErr(ctx, log, err, "Generating Salt Minion Install script for Linux")
	}

	return saltScript, nil
}

func (vdo *VirtualDesktopOrchestrator) buildVirtualMachineSetupScriptLinuxOnline(ctx context.Context, vm *models.VirtualMachine) (string, error) {
	log := logging.GetLogger(ctx)
	cfg := vdo.config
	if cfg.SaltMinionInstall == nil {
		return "", errors.New("Salt Minion install config not provided for Linux VM setup")
	}

	saltScript, err := GenerateInstallSaltMinionAndADJoinOnline(ctx, &cfg)
	if err != nil {
		return "", logging.LogAndWrapErr(ctx, log, err, "Generating Salt Minion Install script for Linux")
	}

	return saltScript, nil
}

func GenerateInstallSaltMinionScriptLinuxOffline(
	ctx context.Context,
	creds *cloudyazure.AzureCredentials,
	storageAccountName, containerName string,
	saltConfig *SaltMinionInstallConfig,
) (string, error) {

	if saltConfig.SaltMaster == "" {
		return "", errors.New("salt master hostname/IP is empty")
	}
	if saltConfig.SaltMinionDebFilename == "" && saltConfig.SaltMinionRpmFilename == "" {
		return "", errors.New("At least one Salt minion package filename (deb or rpm) is required")
	}
	if saltConfig.SaltCommonDebFilename == "" && saltConfig.SaltCommonRpmFilename == "" {
		return "", errors.New("At least one Salt common package filename (deb or rpm) is required")
	}
	if saltConfig.NcalDebFilename == "" {
		return "", errors.New("ncal package filename is required")
	}

	validFor := 1 * time.Hour

	// DEB packages
	var debURLMinion, debURLCommon, bsdDebURL, dctrlDebURL, ncalDebURL string

	if saltConfig.SaltMinionDebFilename != "" {
		u, err := storage.GenerateBlobSAS(
			ctx,
			creds,
			storageAccountName,
			containerName,
			saltConfig.SaltMinionDebFilename,
			validFor,
			sas.BlobPermissions{Read: true},
		)
		if err != nil {
			return "", fmt.Errorf("failed to generate SAS for salt-minion DEB: %w", err)
		}
		debURLMinion = u
	}
	if saltConfig.SaltCommonDebFilename != "" {
		u, err := storage.GenerateBlobSAS(
			ctx,
			creds,
			storageAccountName,
			containerName,
			saltConfig.SaltCommonDebFilename,
			validFor,
			sas.BlobPermissions{Read: true},
		)
		if err != nil {
			return "", fmt.Errorf("failed to generate SAS for salt-common DEB: %w", err)
		}
		debURLCommon = u
	}
	if saltConfig.BsdmainutilsDebFilename != "" {
		u, err := storage.GenerateBlobSAS(
			ctx,
			creds,
			storageAccountName,
			containerName,
			saltConfig.BsdmainutilsDebFilename,
			validFor,
			sas.BlobPermissions{Read: true},
		)
		if err != nil {
			return "", fmt.Errorf("failed to generate SAS for bsdmainutils DEB: %w", err)
		}
		bsdDebURL = u
	}
	if saltConfig.DctrlToolsDebFilename != "" {
		u, err := storage.GenerateBlobSAS(
			ctx,
			creds,
			storageAccountName,
			containerName,
			saltConfig.DctrlToolsDebFilename,
			validFor,
			sas.BlobPermissions{Read: true},
		)
		if err != nil {
			return "", fmt.Errorf("failed to generate SAS for dctrl-tools DEB: %w", err)
		}
		dctrlDebURL = u
	}
	if saltConfig.NcalDebFilename != "" {
		u, err := storage.GenerateBlobSAS(
			ctx,
			creds,
			storageAccountName,
			containerName,
			saltConfig.NcalDebFilename,
			validFor,
			sas.BlobPermissions{Read: true},
		)
		if err != nil {
			return "", fmt.Errorf("failed to generate SAS for ncal DEB: %w", err)
		}
		ncalDebURL = u
	}

	// RPM packages
	var rpmURLMinion, rpmURLCommon string
	if saltConfig.SaltMinionRpmFilename != "" {
		u, err := storage.GenerateBlobSAS(
			ctx,
			creds,
			storageAccountName,
			containerName,
			saltConfig.SaltMinionRpmFilename,
			validFor,
			sas.BlobPermissions{Read: true},
		)
		if err != nil {
			return "", fmt.Errorf("failed to generate SAS for salt-minion RPM: %w", err)
		}
		rpmURLMinion = u
	}
	if saltConfig.SaltCommonRpmFilename != "" {
		u, err := storage.GenerateBlobSAS(
			ctx,
			creds,
			storageAccountName,
			containerName,
			saltConfig.SaltCommonRpmFilename,
			validFor,
			sas.BlobPermissions{Read: true},
		)
		if err != nil {
			return "", fmt.Errorf("failed to generate SAS for salt-common RPM: %w", err)
		}
		rpmURLCommon = u
	}

	// Generate the script with the SAS URLs
	script := installSaltMinionLinuxTemplate

	replacements := map[string]string{
		"$AZURE_SALT_MINION_DEB_URL":  debURLMinion,
		"$AZURE_SALT_COMMON_DEB_URL":  debURLCommon,
		"$AZURE_BSDMAINUTILS_DEB_URL": bsdDebURL,
		"$AZURE_DCTRL_TOOLS_DEB_URL":  dctrlDebURL,
		"$AZURE_NCAL_DEB_URL":         ncalDebURL,

		"$AZURE_SALT_MINION_RPM_URL": rpmURLMinion,
		"$AZURE_SALT_COMMON_RPM_URL": rpmURLCommon,
		"$SALT_MASTER":               saltConfig.SaltMaster,
	}

	for placeholder, value := range replacements {
		script = strings.ReplaceAll(script, placeholder, value)
	}

	return script, nil
}


// GenerateLinuxBootstrapScript builds a single bash script:
// 1) AD join block  (if cfg.AD != nil)
// 2) Salt-minion install block
func GenerateInstallSaltMinionAndADJoinOnline(ctx context.Context, cfg *VirtualDesktopOrchestratorConfig) (string, error) {
    if cfg == nil {
        return "", errors.New("nil config")
    }

    var sb strings.Builder
    sb.WriteString(shellHeader)

    // ── AD block (optional) ────────────────────────────────────────────────
    if cfg.AD != nil {

		// strip DOMAIN\ or DOMAIN/ prefix
		strippedUser := cfg.AD.DomainUsername
		if idx := strings.IndexAny(strippedUser, "\\/"); idx != -1 {
			strippedUser = strippedUser[idx+1:]
		}

        ou := stringPtrOrEmpty(cfg.AD.OrganizationalUnitPath)
        dc := firstOrEmpty(cfg.VM.DomainControllers)

        ad := adJoinBlock
        for k, v := range map[string]string{
            "$AD_DOMAIN": cfg.AD.DomainName,
            "$AD_USER":   strippedUser,
            "$AD_PASS":   cfg.AD.DomainPassword,
            "$AD_OU":     ou,
            "$AD_DC":     dc,
        } {
            ad = strings.ReplaceAll(ad, k, v)
        }
        sb.WriteString(ad)
    }

    // ── Salt block (always) ────────────────────────────────────────────────
    if cfg.SaltMinionInstall != nil && cfg.SaltMinionInstall.SaltMaster != "" {
		salt := strings.ReplaceAll(saltBlock, "$SALT_MASTER", cfg.SaltMinionInstall.SaltMaster)
		sb.WriteString(salt)
    }

    sb.WriteString(shellFooter)
    return sb.String(), nil
}

func stringPtrOrEmpty(p *string) string {
    if p == nil {
        return ""
    }
    return *p
}

func firstOrEmpty(ptrs []*string) string {
    if len(ptrs) == 0 || ptrs[0] == nil {
        return ""
    }
    return *ptrs[0]
}


// ───────────────────────── shellHeader ─────────────────────────
const shellHeader = `#!/usr/bin/env bash
set -euo pipefail

LOG=/var/log/ark-init.log
exec 3>&1                       # anything we want Azure to show
exec 1>>"$LOG" 2>&1             # everything else into a file

fatal(){ printf '[%s] [ERROR] %s\n' "$(date +'%F %T')" "$*" >&3; exit 1; }
trap 'fatal "line $LINENO – aborted."' ERR
log(){ printf '[%s] [INFO] %s\n' "$(date +'%F %T')" "$*" >&3; }
`

// ───────────────────────── adJoinBlock ─────────────────────────
const adJoinBlock = `
# ── Stage 1 : Active-Directory join ──────────────────────────────────────────

# ----- INSTALL DEPENDENCIES -----
log "Installing required packages..."
if command -v apt-get &>/dev/null; then
  DEBIAN_FRONTEND=noninteractive apt-get -qq update
  DEBIAN_FRONTEND=noninteractive apt-get -yqq install \
      realmd sssd sssd-tools adcli oddjob oddjob-mkhomedir \
      samba-common-bin packagekit dnsutils expect krb5-user
else
  dnf -qy install realmd sssd adcli oddjob oddjob-mkhomedir \
                  samba-common-tools bind-utils expect krb5-workstation
fi
log "Package installation complete."

# ----- CONFIGURATION -----
DOMAIN_NAME="$AD_DOMAIN"
DOMAIN_USER="$AD_USER"
DOMAIN_PASSWORD="$AD_PASS"
OU_PATH="$AD_OU"
DC_IP="$AD_DC"

# ----- DERIVED VARIABLES -----
FQDN_LOWER=$(hostname --fqdn | tr '[:upper:]' '[:lower:]')
SHORT_LOWER=$(hostname -s | tr '[:upper:]' '[:lower:]')
IP_ADDR=$(hostname -I | awk '{print $1}')
REALM_UPPER=$(echo "$DOMAIN_NAME" | tr '[:lower:]' '[:upper:]')

# ----- HOSTNAME & /etc/hosts PATCH -----
log "Setting hostname to $FQDN_LOWER and patching /etc/hosts"
sudo hostnamectl set-hostname "$FQDN_LOWER"

sudo sed -i "/\s$SHORT_LOWER$/Id" /etc/hosts
if ! grep -qi "$FQDN_LOWER" /etc/hosts; then
    echo "$IP_ADDR $FQDN_LOWER $SHORT_LOWER" | sudo tee -a /etc/hosts > /dev/null
    log "Added $FQDN_LOWER to /etc/hosts"
else
    log "Entry for $FQDN_LOWER already exists in /etc/hosts"
fi

# ----- DYNAMIC DNS UPDATE IF RECORD IS MISSING -----
log "Checking DNS for $FQDN_LOWER"
if ! host "$FQDN_LOWER" > /dev/null 2>&1; then
    log "DNS record missing, registering via nsupdate"
    echo "$DOMAIN_PASSWORD" | kinit "$DOMAIN_USER@$REALM_UPPER"
    nsupdate -g <<EOF
server $DC_IP
update delete $FQDN_LOWER A
update add $FQDN_LOWER 3600 A $IP_ADDR
send
EOF
    log "DNS record updated for $FQDN_LOWER"
else
    log "DNS record already exists for $FQDN_LOWER"
fi

# ----- ENSURE DOMAIN BASE NAME RESOLVES -----
if ! getent hosts "$DOMAIN_NAME" &>/dev/null; then
  echo "$DC_IP $DOMAIN_NAME" | sudo tee -a /etc/hosts > /dev/null
  log "Patched /etc/hosts for domain $DOMAIN_NAME → $DC_IP"
fi

# ----- PRE-CHECKS -----
if ! command -v realm &> /dev/null; then
    fatal "'realmd' is not installed."
fi

# ----- DOMAIN DISCOVERY -----
log "Discovering domain: $DOMAIN_NAME"
if ! realm discover "$DOMAIN_NAME"; then
    fatal "Failed to discover domain."
fi
log "Domain $DOMAIN_NAME discovered successfully."

# ----- JOINING DOMAIN -----
if realm list | grep -qi "$DOMAIN_NAME"; then
  log "Already joined to domain: $DOMAIN_NAME"
else
  log "Joining domain $DOMAIN_NAME as $DOMAIN_USER"
  expect <<EOF
spawn realm join -v -U "$DOMAIN_USER" --computer-name=$SHORT_LOWER ${OU_PATH:+--computer-ou=$OU_PATH} "$DOMAIN_NAME"
expect "Password for *:"
send "$DOMAIN_PASSWORD\r"
expect eof
EOF

  if realm list | grep -qi "$DOMAIN_NAME"; then
    log "Successfully joined domain: $DOMAIN_NAME"
  else
    fatal "Failed to join domain."
  fi
fi

log "Stage 1 – AD join complete."
`

// ───────────────────────── saltBlock ──────────────────────────
const saltBlock = `
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
`

// ───────────────────────── shellFooter ────────────────────────
const shellFooter = `
log "Bootstrap finished OK"
`


// Air-gapped install using individual packages in storage account
var installSaltMinionLinuxTemplate = `#!/usr/bin/env bash
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
`
