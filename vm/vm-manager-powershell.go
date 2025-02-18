package vm

import (
	"context"
	"fmt"

	powershell "github.com/appliedres/cloudy-azure/powershell/vm-setup"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
)

func (vmm *AzureVirtualMachineManager) ExecuteSetupPowershell(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "ExecuteSetupPowershell started")

	// TODO: generate SAS URL
	// perms := sas.ContainerPermissions{Read: true, List: true}
	// validFor := 1*time.Hour

	// storageAccountName := avd.config.StorageAccountName
	// containerName := avd.config.ContainerName

	// sasURL, err := storage.GenerateUserDelegationSAS(ctx, avd.credentials, storageAccountName, containerName, validFor, perms)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to generate SAS token: %w", err)
	// }
	// log.DebugContext(ctx, "Generated SAS token",
	// 	"storageAccount", storageAccountName,
	// 	"container", containerName,
	// 	"validFor", fmt.Sprintf("%d days %d hours %d minutes", int(validFor.Hours()/24), int(validFor.Hours())%24, int(validFor.Minutes())%60),
	// 	"permissions", perms)

	// VM create complete, begin VM setup
	powershellConfig := powershell.PowershellConfig{
		ADJoin: &powershell.ADJoinConfig{
			DomainName: "",
		},

		// EnableADJoin: true,  // TODO: Handle AD join disable
		// EnableAVDInstall: true,
		// EnableSaltInstall: true,  // TODO: Handle salt install disable
		RestartVirtualMachine: true, // TODO: Handle VM restart disable
	}

	if vmm.avdManager == nil {
		log.InfoContext(ctx, "Executing powershell - AVD disabled")
		// AVD Disabled via config
		script, err := powershell.BuildVirtualMachineSetupScript(powershellConfig)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "Could not build powershell script (AVD disabled)")
		}

		err = vmm.ExecuteRemotePowershell(ctx, vm.ID, script)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "Could not run powershell (AVD disabled)")

		}

	} else {
		log.InfoContext(ctx, "Executing powershell - AVD enabled")
		// AVD Enabled
		hostPoolNamePtr, hostPoolTokenPtr, err := vmm.avdManager.PreRegister(ctx, vm)
		powershellConfig.AVDInstall.HostPoolRegistrationToken = *hostPoolTokenPtr
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "AVD Pre-Register failed")
		}

		script, err := powershell.BuildVirtualMachineSetupScript(powershellConfig)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "Could not build powershell script (AVD enabled)")
		}

		err = vmm.ExecuteRemotePowershell(ctx, vm.ID, script)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "Could not run powershell (AVD enabled)")
		}

		vm, err = vmm.avdManager.PostRegister(ctx, vm, *hostPoolNamePtr)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "AVD Post-Register VM")
		}
	}

	return vm, nil
}

// getOptionalConfig retrieves the config value. If the value is nil or empty, it returns the fallback value.
func getOptionalConfig(ctx context.Context, configValue *string, fallbackValue string) string {
	log := logging.GetLogger(ctx)

	// If the actual config value is present and non-empty, return it.
	if configValue != nil && *configValue != "" {
		log.DebugContext(ctx, fmt.Sprintf("Returning provided config value '%s'.", *configValue))
		return *configValue
	}

	// Otherwise, check if we have a valid fallback.
	log.DebugContext(ctx, fmt.Sprintf("config value not provided; using fallback value '%s'.", fallbackValue))
	return fallbackValue
}
