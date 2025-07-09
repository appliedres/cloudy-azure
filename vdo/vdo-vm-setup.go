package vdo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "embed"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy-azure/storage"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
)

//go:embed vm-setup-shell/shellHeader.sh
var shellHeader string

//go:embed vm-setup-shell/adJoinBlock.sh
var adJoinBlock string

//go:embed vm-setup-shell/saltBlock.sh
var saltBlock string

//go:embed vm-setup-shell/rdpSetupBlock.sh
var rdpSetupBlock string

//go:embed vm-setup-shell/shellFooter.sh
var shellFooter string

//go:embed vm-setup-shell/installSaltMinionLinuxTemplateOffline.sh
var installSaltMinionLinuxTemplateOffline string

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

	// // write out to a file for testing
	// timestamp := time.Now().Format("20060102-150405")
	// outPath := fmt.Sprintf("/tmp/bootstrap-%s.sh", timestamp)
	//     if err := os.WriteFile(outPath, []byte(script), 0o700); err != nil {
	//     return "", fmt.Errorf("failed writing bootstrap script to %s: %w", outPath, err)
	// }

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
