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

func (vdo *VirtualDesktopOrchestrator) InitialVirtualMachineSetup(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "InitialVirtualMachineSetup started")

	var err error
	switch vm.Template.OperatingSystem {
	case "windows":
		vm, err = vdo.virtualMachineSetupWindows(ctx, vm)
	case "linux":
		vm, err = vdo.virtualMachineSetupLinux(ctx, vm)
	default:
		err = errors.New("unsupported operating system during initial VM setup")
	}

    if err != nil {
	    return vm, logging.LogAndWrapErr(ctx, log, err, "initial VM setup failed")
    }
    return vm, nil
}

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

	shellScript, err := vdo.buildVirtualMachineSetupScriptLinux(ctx, vm)
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

func (vdo *VirtualDesktopOrchestrator) buildVirtualMachineSetupScriptLinux(ctx context.Context, vm *models.VirtualMachine) (string, error) {
	log := logging.GetLogger(ctx)
	cfg := vdo.config
	if cfg.SaltMinionInstall == nil {
		return "", errors.New("Salt Minion install config not provided for Linux VM setup")
	}

	saltScript, err := GenerateInstallSaltMinionScriptLinux(
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

func GenerateInstallSaltMinionScriptLinux(
	ctx context.Context,
	creds *cloudyazure.AzureCredentials,
	storageAccountName, containerName string,
	saltConfig *SaltMinionInstallConfig,
) (string, error) {

	if saltConfig.SaltMaster == "" {
		return "", errors.New("Salt master hostname/IP is empty")
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
