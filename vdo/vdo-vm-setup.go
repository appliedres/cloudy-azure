package vdo

import (
	"context"
	"errors"
	"fmt"
	"os"
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

		script, err := vdo.buildSetupScriptWindows(ctx, vdoConfig, hostPoolToken)
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
		script, err := vdo.buildSetupScriptWindows(ctx, vdoConfig, nil)
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
	if saltConfig.SaltCommonDebFilename == "" && saltConfig.SaltBaseRpmFilename == "" {
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
		"$AZURE_SALT_MINION_DEB_URL": debURLMinion,
		"$AZURE_SALT_COMMON_DEB_URL": debURLCommon,
		"$AZURE_BSDMAIN_DEB_URL":     bsdMainDebURL,
		"$AZURE_BSDEXTRA_DEB_URL":    bsdExtraDebURL,
		"$AZURE_DCTRL_TOOLS_DEB_URL": dctrlDebURL,
		"$AZURE_NCAL_DEB_URL":        ncalDebURL,

		"$AZURE_SALT_MINION_RPM_URL": rpmURLMinion,
		"$AZURE_SALT_BASE_RPM_URL":   rpmURLSalt,

		"$SALT_MASTER": saltConfig.SaltMaster,
	}

	for placeholder, value := range replacements {
		script = strings.ReplaceAll(script, placeholder, value)
	}

	return script, nil
}

// stripADUsername removes the AD domain prefix or suffix from a full username.
func stripADUsername(ctx context.Context, fullUsername string) string {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "stripADUsername started", "fullUsername", fullUsername)

	var stripped = fullUsername

	// Strip prefix: DOMAIN\username or DOMAIN/username
	if idx := strings.IndexAny(stripped, "\\/"); idx != -1 {
		stripped = stripped[idx+1:]
	}

	// Strip suffix: username@domain.com
	if atIdx := strings.Index(stripped, "@"); atIdx != -1 {
		stripped = stripped[:atIdx]
	}

	// Final validation: no empty string, no backslash, slash, or @
	if strings.TrimSpace(stripped) == "" || strings.ContainsAny(stripped, "\\/@") {
		log.WarnContext(ctx, "Invalid AD username format after stripping. using it anyways...", "input", fullUsername, "stripped", stripped)
		return stripped
	}

	log.DebugContext(ctx, "stripADUsername complete", "stripped", stripped)
	return stripped
}

// GenerateLinuxBootstrapScript builds a single bash script:
// 1) AD join block  (if cfg.AD != nil)
// 2) Salt-minion install block
func GenerateInstallSaltMinionAndADJoinOnline(ctx context.Context, cfg *VirtualDesktopOrchestratorConfig) (string, error) {
    log := logging.GetLogger(ctx)
    
    if cfg == nil {
        return "", errors.New("nil config")
    }

    var sb strings.Builder
    sb.WriteString(shellHeader)

    // AD block
    if cfg.AD != nil {

        ou := stringPtrOrEmpty(cfg.AD.OrganizationalUnitPath)
        dc := firstOrEmpty(cfg.VM.DomainControllers)
        if dc == "" {
            dc = "10.0.130.4" // FIXME: don't do this.. unless it's the night before a demo
        }

        strippedUser := stripADUsername(ctx, cfg.AD.DomainUsername)
        if strippedUser == "" {
            return "", errors.New("AD username cannot be empty after stripping domain prefix/suffix")
        }

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
        log.DebugContext(ctx, "AD join block added to script", "Domain", cfg.AD.DomainName, "USER", strippedUser, "OU", ou, "DC", dc)
    }

	// RDP setup block (always)
	sb.WriteString(rdpSetupBlock)

    // Salt block
    if cfg.SaltMinionInstall != nil && cfg.SaltMinionInstall.SaltMaster != "" {
		salt := strings.ReplaceAll(saltBlock, "$SALT_MASTER", cfg.SaltMinionInstall.SaltMaster)
		sb.WriteString(salt)
    }

    sb.WriteString(shellFooter)

	script := sb.String()

    // write out to a file for testing
	timestamp := time.Now().Format("20060102-150405")
    outPath := fmt.Sprintf("/tmp/bootstrap-%s.sh", timestamp)
	    if err := os.WriteFile(outPath, []byte(script), 0o700); err != nil {
        return "", fmt.Errorf("failed writing bootstrap script to %s: %w", outPath, err)
    }

    return script, nil
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

on_error() {
  local exit_code=$?
  local line_no=$1
  local cmd=$2
  printf '[%s] [ERROR] line %d → "%s" (exit %d)\n' \
         "$(date +'%F %T')" "$line_no" "$cmd" "$exit_code" >&3
  exit "$exit_code"
}
set -o errtrace           # let ERR propagate through functions/ subshells
trap 'on_error $LINENO "$BASH_COMMAND"' ERR

log(){ printf '[%s] [INFO] %s\n' "$(date +'%F %T')" "$*" >&3; }

`

// ───────────────────────── adJoinBlock ─────────────────────────
const adJoinBlock = `
# ── Stage 1 : AD Domain Join ────────────────────────────────────────────────

# replaced via go
AD_DOMAIN="$AD_DOMAIN"
AD_USER="$AD_USER"
AD_PASS="$AD_PASS"
AD_OU="$AD_OU"
AD_DC="$AD_DC"

# generated
AD_REALM=$(echo "$AD_DOMAIN" | tr '[:lower:]' '[:upper:]')
VM_NAME=$(hostname -s | tr '[:upper:]' '[:lower:]')

install_ad_packages() {
  log "Installing packages for AD join..."
  if [ -f /etc/os-release ]; then
    . /etc/os-release
  else
    log "[ERROR] Cannot find /etc/os-release; aborting."
    exit 1
  fi

  case "$ID" in
    debian|ubuntu)
      export DEBIAN_FRONTEND=noninteractive
      apt-get update -y
      apt-get install -y realmd sssd sssd-tools adcli oddjob oddjob-mkhomedir \
        samba-common-bin packagekit dnsutils expect krb5-user
      ;;
    rhel|centos|rocky|almalinux|fedora)
      yum install -y realmd sssd sssd-tools adcli oddjob oddjob-mkhomedir \
        samba-common-tools expect krb5-workstation
      ;;
    *)
      log "[ERROR] Unsupported distribution: $ID"
      exit 1
      ;;
  esac
}

configure_dns() {
  # ------------------------------------------------------------------------- #
  # Best‑effort DNS setup – prefers systemd‑resolved (resolvectl)             #
  # ------------------------------------------------------------------------- #
  # • Validates / discovers $AD_DC and $AD_DOMAIN                             #
  # • Uses resolvectl when available & active                                 #
  # • Falls back to writing /etc/resolv.conf                                  #
  # • Never exits the caller; only logs WARN on failure                       #
  # ------------------------------------------------------------------------- #
  log "Configuring DNS (prefer resolvectl) …"

  # pick the first default‑route interface
  IFACE=$(ip route | awk '/^default/ {print $5; exit}')
  if [[ -z $IFACE ]]; then
    log "[WARN] No default route found – skipping DNS setup"
    return
  fi

  # ----- ensure we have a DNS server ---------------------------------------
  if [[ -z ${AD_DC:-} ]]; then
    log "[WARN] AD_DC is empty – trying SRV discovery for ${AD_DOMAIN:-<unset>}"
    if command -v dig &>/dev/null && [[ -n ${AD_DOMAIN:-} ]]; then
      AD_DC=$(dig +short _ldap._tcp."$AD_DOMAIN" SRV | awk '{print $4; exit}')
      [[ -n $AD_DC ]] && log "Discovered DC via SRV: $AD_DC"
    fi
  fi

  # ----- try resolvectl -----------------------------------------------------
  if command -v resolvectl &>/dev/null && systemctl is-active --quiet systemd-resolved; then
    [[ -n $AD_DC ]]     && resolvectl dns    "$IFACE" "$AD_DC"     \
                           && log "resolvectl: $IFACE → DNS $AD_DC" \
                           || log "[WARN] resolvectl dns failed (AD_DC='$AD_DC')"
    [[ -n $AD_DOMAIN ]] && resolvectl domain "$IFACE" "$AD_DOMAIN" \
                           && log "resolvectl: $IFACE → search $AD_DOMAIN" \
                           || log "[WARN] resolvectl domain failed (AD_DOMAIN='$AD_DOMAIN')"

    # If at least one of the resolvectl calls succeeded we're done
    [[ -n $AD_DC || -n $AD_DOMAIN ]] && return
  else
    log "[WARN] resolvectl unavailable or systemd‑resolved inactive – falling back to /etc/resolv.conf"
  fi

  # ----- final fallback -----------------------------------------------------
  {
    [[ -n $AD_DC     ]] && printf 'nameserver %s\n' "$AD_DC"
    [[ -n $AD_DOMAIN ]] && printf 'search %s\n'     "$AD_DOMAIN"
  } | sudo tee /etc/resolv.conf >/dev/null

  log "Updated /etc/resolv.conf directly (may be overwritten by NetworkManager)"
}

join_ad_domain() {
  log "Checking if already joined to $AD_DOMAIN..."
  if realm list | grep -q "^$AD_DOMAIN\$"; then
    log "Already joined to $AD_DOMAIN. Skipping join."
    return
  fi

  log "Joining domain $AD_DOMAIN with computer name $VM_NAME"
  expect <<EOF
spawn realm join -v -U "$AD_USER" --computer-name=$VM_NAME ${AD_OU:+--computer-ou=$AD_OU} "$AD_DOMAIN"
expect "Password for *:"
send "$AD_PASS\r"
expect eof
EOF

  log "Validating that the system is now joined to $AD_DOMAIN..."
  if realm list | grep -q "^$AD_DOMAIN\$"; then
    log "Successfully joined to $AD_DOMAIN"
  else
    log "[ERROR] Domain join appears to have failed — $AD_DOMAIN not listed in realm list"
    exit 1
  fi
}

patch_sssd_conf() {
  local SSSD_CONF="/etc/sssd/sssd.conf"
  log "Patching $SSSD_CONF"

  if [ ! -f "$SSSD_CONF" ]; then
    mkdir -p /etc/sssd
    touch "$SSSD_CONF"
    echo -e "[sssd]\ndomains = $AD_DOMAIN\nconfig_file_version = 2\nservices = nss, pam\n\n[domain/$AD_DOMAIN]" > "$SSSD_CONF"
  fi

  chmod 600 "$SSSD_CONF"

  # Clean conflicting settings
  sed -i '/^access_provider/d' "$SSSD_CONF"
  sed -i '/^ad_access_filter/d' "$SSSD_CONF"
  sed -i '/^simple_allow_users/d' "$SSSD_CONF"
  sed -i '/^use_fully_qualified_names/d' "$SSSD_CONF"
  sed -i '/^fallback_homedir/d' "$SSSD_CONF"
  sed -i '/^default_shell/d' "$SSSD_CONF"

  # Insert enforced access configuration
  sed -i "/^\[domain\/.*\]/a access_provider = permit" "$SSSD_CONF"

  # Insert default options if not already present
  echo "use_fully_qualified_names = false" >> "$SSSD_CONF"
  echo "fallback_homedir = /home/%u" >> "$SSSD_CONF"
  echo "default_shell = /bin/bash" >> "$SSSD_CONF"

  systemctl restart sssd
  log "$SSSD_CONF patched with access_provider=permit (unrestricted AD logins)"
}

verify_ad_status() {
  log "Verifying AD domain join status..."

  # Check if realm list includes the domain
  if realm list | grep -q "$AD_DOMAIN"; then
    log "realm list includes $AD_DOMAIN"
  else
    log "[ERROR] realm list does not include $AD_DOMAIN"
    exit 1
  fi

  # Check sssctl domain status is online
  if sssctl domain-status "$AD_DOMAIN" | grep -q "Online status: Online"; then
    log "SSSD reports domain $AD_DOMAIN is Online"
  else
    log "[ERROR] SSSD domain status is not Online for $AD_DOMAIN"
    sssctl domain-status "$AD_DOMAIN"
    exit 1
  fi

  # Check user can be resolved without FQDN suffix
  TEST_USER="${AD_USER,,}"
  if getent passwd "$TEST_USER" > /dev/null; then
    log "User $TEST_USER is resolvable without domain suffix"
  else
    log "[ERROR] getent could not resolve user $TEST_USER (without domain suffix)"
    log "Attempting getent with FQDN: $TEST_USER@$AD_DOMAIN"
    getent passwd "$TEST_USER@$AD_DOMAIN" || true
    exit 1
  fi
}

# ── Execute AD join sequence ────────────────────────────────────────────────
install_ad_packages
configure_dns
join_ad_domain
patch_sssd_conf
sleep 5  # time to stabilize
verify_ad_status
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

const rdpSetupBlock = `
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
`


// ───────────────────────── shellFooter ────────────────────────
const shellFooter = `
log "Bootstrap finished OK"
`


// Air-gapped install using individual packages in storage account
var installSaltMinionLinuxTemplateOffline = `#!/usr/bin/env bash
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
