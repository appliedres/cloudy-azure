package vdo

import (
	"context"

	cloudyazure "github.com/appliedres/cloudy-azure"
	cloudyvm "github.com/appliedres/cloudy/vm"

	"github.com/appliedres/cloudy-azure/avd"
	"github.com/appliedres/cloudy-azure/vm"
)

// The VirtualDesktopOrchestrator manages both Azure Virtual Machines and Azure Virtual Desktop (AVD) resources.
// It presents a unified interface for creating and managing virtual machines.
// The management of AVD resources is hidden behind the scenes, and is only used when completing VM operations.
// AVD can be disabled by setting the AVDManagerConfig to nil in the config.
type VirtualDesktopOrchestrator struct {
	name string // for identification when multiple orchestrators are used

	config VirtualDesktopOrchestratorConfig

	vmManager  vm.AzureVirtualMachineManager
	avdManager *avd.AzureVirtualDesktopManager // optional
}

// TODO: how much should credentials match? Do we allow different subscription?

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

	var avdMgr *avd.AzureVirtualDesktopManager
	if isAVDEnabled(*config) {
		avdMgr, err = avd.NewAzureVirtualDesktopManager(ctx, name, avdCredentials, &config.AVD.AVDManagerConfig)
		if err != nil {
			return nil, err
		}
	}

	vdo := &VirtualDesktopOrchestrator{
		name: name,

		config: *config,

		vmManager:  *vmMgr,
		avdManager: avdMgr,
	}

	return vdo, nil
}
