package vm

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

func (vmm *AzureVirtualMachineManager) InitialVirtualMachineSetup(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "InitialVirtualMachineSetup started")

	var err error
	switch vm.Template.OperatingSystem {
	case models.VirtualMachineTemplateOperatingSystemWindows:
		vm, err = vmm.virtualMachineSetupWindows(ctx, vm)
	case models.VirtualMachineTemplateOperatingSystemLinuxDeb, models.VirtualMachineTemplateOperatingSystemLinuxRhel:
		vm, err = vmm.virtualMachineSetupLinux(ctx, vm)
	default:
		err = errors.New("unsupported operating system during initial VM setup")
	}

	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "initial VM setup failed")
	}

	return vm, nil
}

func (vmm *AzureVirtualMachineManager) virtualMachineSetupWindows(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "virtualMachineSetupWindows started")
	defer log.DebugContext(ctx, "virtualMachineSetupWindows finished")

	setupScriptConfig := vmm.config.InitialSetupConfig
	if vmm.avdManager != nil {
		log.InfoContext(ctx, "Initial VM setup - AVD enabled")

		hostPoolNamePtr, hostPoolToken, err := vmm.avdManager.PreRegister(ctx, vm)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "AVD Pre-Register failed")
		}

		script, err := vmm.buildVirtualMachineSetupScript(ctx, *setupScriptConfig, hostPoolToken)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "Could not build powershell script (AVD enabled)")
		}

		err = vmm.ExecuteRemotePowershell(ctx, vm.ID, script, 20*time.Minute, 15*time.Second)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "Could not run powershell (AVD enabled)")
		}

		vm, err = vmm.avdManager.PostRegister(ctx, vm, *hostPoolNamePtr)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "AVD Post-Register VM")
		}

	} else {
		log.InfoContext(ctx, "Initial VM setup - AVD disabled")
		script, err := vmm.buildVirtualMachineSetupScript(ctx, *setupScriptConfig, nil)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "Could not build powershell script (AVD disabled)")
		}

		err = vmm.ExecuteRemotePowershell(ctx, vm.ID, script, 10*time.Minute, 15*time.Second)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "Could not run powershell (AVD disabled)")

		}
	}

	log.InfoContext(ctx, "Initial Windows VM setup completed successfully")
	return vm, nil
}

func (vmm *AzureVirtualMachineManager) virtualMachineSetupLinux(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "virtualMachineSetupLinux started")
	defer log.DebugContext(ctx, "virtualMachineSetupLinux finished")

	shellScript, err := vmm.buildVirtualMachineSetupScriptLinuxOffline(ctx, vm)
	if err != nil {
		log.WarnContext(ctx, "failed to build Linux setup script for offline install, skipping....", logging.WithError(err))
		return nil, nil
	}

	err = vmm.ExecuteRemoteShellScript(ctx, vm.ID, &shellScript, 10*time.Minute, 15*time.Second)
	if err != nil {
		log.WarnContext(ctx, "failed to execute Linux setup script for offline install, skipping....", logging.WithError(err))
		return nil, nil
	}

	log.InfoContext(ctx, "Initial Linux VM setup completed successfully")
	return vm, nil
}

func (vmm *AzureVirtualMachineManager) buildVirtualMachineSetupScriptLinuxOnline(ctx context.Context, vm *models.VirtualMachine) (string, error) {
	log := logging.GetLogger(ctx)
	cfg := vmm.config.InitialSetupConfig
	if cfg == nil || cfg.SaltMinionInstallConfig == nil {
		return "", errors.New("Salt Minion install config not provided for Linux VM setup")
	}

	saltScript, err := GenerateInstallSaltMinionScriptLinux(
		ctx,
		vmm.credentials,
		cfg.BinaryStorage.BlobStorageAccount,
		cfg.BinaryStorage.BlobContainer,
		cfg.SaltMinionInstallConfig,
	)
	if err != nil {
		return "", logging.LogAndWrapErr(ctx, log, err, "Generating Salt Minion Install script for Linux")
	}

	return saltScript, nil
}

func GenerateInstallSaltMinionScriptLinux(
	ctx context.Context,
	creds *cloudyazure.AzureCredentials,
	storageAccountName, containerName string,
	saltConfig *SaltMinionInstallConfig,
) (string, error) {

	if saltConfig.SaltMaster == "" {
		return "", errors.New("salt master hostname/IP is empty")
	}

	// Single placeholder in the new template
	script := installSaltMinionOnlineTemplate
	script = strings.ReplaceAll(script, "$SALT_MASTER", saltConfig.SaltMaster)

	return script, nil
}

var installSaltMinionOnlineTemplate = `#!/usr/bin/env bash
set -euo pipefail
trap 'echo "[ERROR] $LINENO: command failed." >&2; exit 1' ERR

log() { echo "[INFO] $*"; }

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


////////////////////////////////////////////
// Offline Install
////////////////////////////////////////////

func (vmm *AzureVirtualMachineManager) buildVirtualMachineSetupScriptLinuxOffline(ctx context.Context, vm *models.VirtualMachine) (string, error) {
	log := logging.GetLogger(ctx)
	cfg := vmm.config
	if cfg.InitialSetupConfig.SaltMinionInstallConfig == nil {
		log.WarnContext(ctx, "Salt Minion install config not provided for Linux VM setup, skipping Salt Minion installation")
		return "", nil
	}

	cfg.InitialSetupConfig.SaltMinionInstallConfig.SaltMinionRpmFilename = 	"salt-minion.rpm"
	cfg.InitialSetupConfig.SaltMinionInstallConfig.SaltBaseRpmFilename = 	"salt.rpm"
	
	cfg.InitialSetupConfig.SaltMinionInstallConfig.SaltMinionDebFilename = "salt-minion.deb"
	cfg.InitialSetupConfig.SaltMinionInstallConfig.SaltCommonDebFilename = "salt-common.deb"
	cfg.InitialSetupConfig.SaltMinionInstallConfig.BsdmainDebFilename = "bsdmain.deb"
	cfg.InitialSetupConfig.SaltMinionInstallConfig.BsdextraDebFilename = "bsdextra.deb"
	cfg.InitialSetupConfig.SaltMinionInstallConfig.DctrlToolsDebFilename = "dctrl.deb"
	cfg.InitialSetupConfig.SaltMinionInstallConfig.NcalDebFilename = "ncal.deb"

	saltScript, err := GenerateInstallSaltMinionScriptLinuxOffline(
		ctx,
		vmm.credentials,
		cfg.InitialSetupConfig.BinaryStorage.BlobStorageAccount,
		cfg.InitialSetupConfig.BinaryStorage.BlobContainer,
		cfg.InitialSetupConfig.SaltMinionInstallConfig,
	)
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
	if saltConfig.SaltCommonDebFilename == "" {
		return "", errors.New("At least one Salt common package filename (deb or rpm) is required")
	}
	if saltConfig.NcalDebFilename == "" {
		return "", errors.New("ncal package filename is required")
	}

	validFor := 1 * time.Hour

	// DEB packages
	var debURLMinion, debURLCommon, bsdMainDebURL, bsdExtraDebURL, dctrlDebURL, ncalDebURL string


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
	if saltConfig.BsdmainDebFilename != "" {
		u, err := storage.GenerateBlobSAS(
			ctx,
			creds,
			storageAccountName,
			containerName,
			saltConfig.BsdmainDebFilename,
			validFor,
			sas.BlobPermissions{Read: true},
		)
		if err != nil {
			return "", fmt.Errorf("failed to generate SAS for bsdmainutils DEB: %w", err)
		}
		bsdMainDebURL = u
	}
	if saltConfig.BsdextraDebFilename != "" {
		u, err := storage.GenerateBlobSAS(
			ctx,
			creds,
			storageAccountName,
			containerName,
			saltConfig.BsdextraDebFilename,
			validFor,
			sas.BlobPermissions{Read: true},
		)
		if err != nil {
			return "", fmt.Errorf("failed to generate SAS for bsdmainutils DEB: %w", err)
		}
		bsdExtraDebURL = u
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
	// var rpmURLMinion string
	var rpmURLMinion, rpmURLSalt string
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
	if saltConfig.SaltBaseRpmFilename != "" {
		u, err := storage.GenerateBlobSAS(
			ctx,
			creds,
			storageAccountName,
			containerName,
			saltConfig.SaltBaseRpmFilename,
			validFor,
			sas.BlobPermissions{Read: true},
		)
		if err != nil {
			return "", fmt.Errorf("failed to generate SAS for salt-common RPM: %w", err)
		}
		rpmURLSalt = u
	}

	// Generate the script with the SAS URLs
	script := installSaltMinionLinuxTemplateOffline

	replacements := map[string]string{
		"$AZURE_SALT_MINION_DEB_URL":  	debURLMinion,
		"$AZURE_SALT_COMMON_DEB_URL":  	debURLCommon,
		"$AZURE_BSDMAIN_DEB_URL": 		bsdMainDebURL,
		"$AZURE_BSDEXTRA_DEB_URL": 		bsdExtraDebURL,
		"$AZURE_DCTRL_TOOLS_DEB_URL":  	dctrlDebURL,
		"$AZURE_NCAL_DEB_URL":         	ncalDebURL,

		"$AZURE_SALT_MINION_RPM_URL": 	rpmURLMinion,
		"$AZURE_SALT_BASE_RPM_URL": 	rpmURLSalt,
		
		"$SALT_MASTER":               saltConfig.SaltMaster,
	}

	for placeholder, value := range replacements {
		script = strings.ReplaceAll(script, placeholder, value)
	}

	return script, nil
}

// Air-gapped install using individual packages in storage account
var installSaltMinionLinuxTemplateOffline = `#!/usr/bin/env bash
set -euo pipefail

trap 'echo "[ERROR] A fatal error occurred at line $LINENO. Exiting." >&2' ERR

log_info()  { echo "[INFO] $*"; }
log_error() { echo "[ERROR] $*" >&2; }

DOWNLOAD_FOLDER="/tmp/ArkloudDownloads"
[ -d "$DOWNLOAD_FOLDER" ] || { log_info "Creating $DOWNLOAD_FOLDER"; mkdir -p "$DOWNLOAD_FOLDER"; }

log_info "Detecting package manager..."
IS_RHEL=false
IS_DEBIAN=false
if command -v dnf >/dev/null 2>&1 || command -v yum >/dev/null 2>&1 || command -v microdnf >/dev/null 2>&1; then
    log_info "Detected RHEL-based system."
    IS_RHEL=true
elif command -v apt-get >/dev/null 2>&1; then
    log_info "Detected Debian-based system."
    IS_DEBIAN=true
else
    log_error "No recognized package manager (dnf/yum/microdnf or apt-get) found. Exiting."
    exit 1
fi

#######################################################################
# Debian / Ubuntu
#######################################################################
if [ "$IS_DEBIAN" = true ]; then
    log_info "Installing DEB packages (offline)…"

    fetch_and_install_deb() {
        local url="$1" outfile="$2"
        [ -n "$url" ] || { log_error "URL missing for $outfile — skipping."; return 1; }
        log_info "Downloading $outfile"
        curl -fSL "$url" -o "$outfile"
        log_info "Installing $outfile"
        if ! dpkg -i "$outfile"; then
            log_info "Fixing dependencies for $outfile"
            apt-get update -y && apt-get install -f -y
        fi
    }

    fetch_and_install_deb "$AZURE_NCAL_DEB_URL"         "$DOWNLOAD_FOLDER/ncal.deb"
	fetch_and_install_deb "$AZURE_BSDEXTRA_DEB_URL" 	"$DOWNLOAD_FOLDER/bsdextra.deb"
    fetch_and_install_deb "$AZURE_BSDMAIN_DEB_URL" 		"$DOWNLOAD_FOLDER/bsdmain.deb"
    fetch_and_install_deb "$AZURE_DCTRL_TOOLS_DEB_URL"  "$DOWNLOAD_FOLDER/dctrl.deb"
    fetch_and_install_deb "$AZURE_SALT_COMMON_DEB_URL"  "$DOWNLOAD_FOLDER/salt-common.deb"
    fetch_and_install_deb "$AZURE_SALT_MINION_DEB_URL"  "$DOWNLOAD_FOLDER/salt-minion.deb"
fi

#######################################################################
# RHEL / CentOS / Rocky / Alma
#######################################################################
if [ "$IS_RHEL" = true ]; then
    log_info "Installing RPM packages (offline)…"

    # Pick the first available package manager
    if command -v dnf >/dev/null 2>&1;      then PKG_CMD="dnf";      elif \
       command -v yum >/dev/null 2>&1;      then PKG_CMD="yum";      else \
       PKG_CMD="microdnf"; fi
    INSTALL_OPTS="-y --nogpgcheck"

    fetch_and_install_rpm() {
        local url="$1" outfile="$2"
        [ -n "$url" ] || { log_error "URL missing for $outfile — skipping."; return 1; }
        log_info "Downloading $outfile"
        curl -fSL "$url" -o "$outfile"
        log_info "Installing $outfile via $PKG_CMD"
        $PKG_CMD install $INSTALL_OPTS "$outfile"
    }

	fetch_and_install_rpm "$AZURE_SALT_BASE_RPM_URL" "$DOWNLOAD_FOLDER/salt.rpm"
    fetch_and_install_rpm "$AZURE_SALT_MINION_RPM_URL" "$DOWNLOAD_FOLDER/salt-minion.rpm"
fi

#######################################################################
# Common post-install steps
#######################################################################
# Update the minion configuration
if [ -f /etc/salt/minion ]; then
    log_info "Updating /etc/salt/minion configuration with master: $SALT_MASTER"
    sed -i '/^[[:space:]]*master:/d' /etc/salt/minion
    echo "master: $SALT_MASTER" | sudo tee -a /etc/salt/minion
else
    log_info "/etc/salt/minion not found. Assuming the package creates it on first run."
fi

# Restart the salt-minion service
log_info "Restarting salt-minion service..."
if command -v systemctl >/dev/null 2>&1; then
    sudo systemctl restart salt-minion || log_info "[ERROR] Failed to restart salt-minion service via systemctl."
else
    sudo service salt-minion restart || log_info "[ERROR] Failed to restart salt-minion service via SysV."
fi

# ── Wait for service to stabilize ──────────────────────────────────────────
sleep 5

# ── Check minion logs for master connection ────────────────────────────────
if sudo grep -qi "Authentication with master at" /var/log/salt/minion; then
  log_info "Salt Minion attempted to connect to master $SALT_MASTER"
else
  log_info "[WARNING] Salt Minion log does not yet show a connection to the master."
fi

# ── Optionally check ping locally (does NOT verify master, but confirms minion works) ──
if sudo salt-call --local test.ping; then
  log_info "Salt Minion installed and working (local test.ping passed)"
else
  log_info "[WARNING] salt-call test.ping failed (local). Check service status and config."
fi

log_info "Salt installation and startup completed successfully!"
`
