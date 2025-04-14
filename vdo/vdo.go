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
	name string // for identification when multiple orchestrators are used

	config VirtualDesktopOrchestratorConfig

	vmManager  vm.AzureVirtualMachineManager
	avdManager *avd.AzureVirtualDesktopManager // optional
}

// TODO: how much should credentials match? Do we allow different tenant, subscription, etc?

func NewVirtualDesktopOrchestrator(
	ctx context.Context,
	name string,
	vmCredentials *cloudyazure.AzureCredentials,
	avdCredentials *cloudyazure.AzureCredentials,
	config *VirtualDesktopOrchestratorConfig,
) (cloudyvm.VirtualDesktopOrchestrator, error) {

	vmmConfig := &config.VM
	vmMgr, err := vm.NewAzureVirtualMachineManager(ctx, name, vmCredentials, vmmConfig)
	if err != nil {
		return nil, err
	}

	avdMgr, err := avd.NewAzureVirtualDesktopManager(ctx, name, avdCredentials, &config.AVD.AVDManagerConfig)
	if err != nil {
		return nil, err
	}

	vdo := &VirtualDesktopOrchestrator{
		name: name,

		config: *config,

		vmManager:  *vmMgr,
		avdManager: avdMgr,
	}

	return vdo, nil
}
