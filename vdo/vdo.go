package vdo

import (
	"context"

	cloudyazure "github.com/appliedres/cloudy-azure"
	cloudyvm "github.com/appliedres/cloudy/vm"

	"github.com/appliedres/cloudy-azure/avd"
	"github.com/appliedres/cloudy-azure/vm"
)

// Manages both VMs and the AVD resources that are associated with them
type VirtualDesktopOrchestrator struct {
	name string  // for identification when multiple orchestrators are used

	credentials cloudyazure.AzureCredentials  // Use a single set of credentials for both, but overwrite resource group for AVD
	config      VirtualDesktopOrchestratorConfig

	vmManager 	vm.AzureVirtualMachineManager
	avdManager 	*avd.AzureVirtualDesktopManager  // optional
}

// TODO: how much should credentials match? Should we use a single set of creds and just overwrite the resource group?

func NewVirtualDesktopOrchestrator(
	ctx context.Context, 
	name string, 
	vmCredentials *cloudyazure.AzureCredentials, 
	avdCredentials *cloudyazure.AzureCredentials, 
	config *VirtualDesktopOrchestratorConfig,
	) (cloudyvm.VirtualDesktopOrchestrator, error) {
    
	vmmConfig := &config.VM
	vmMgr, err := vm.NewAzureVirtualMachineManager(ctx, vmCredentials, vmmConfig)
	if err != nil {
		return nil, err
	}

	avdMgr, err := avd.NewAzureVirtualDesktopManager(ctx, avdCredentials, &config.AVD.AVDManagerConfig)
	if err != nil {
		return nil, err
	}

	vdo := &VirtualDesktopOrchestrator{
        vmManager:  *vmMgr,
        avdManager: avdMgr,
    }
	
	return vdo, nil
}
