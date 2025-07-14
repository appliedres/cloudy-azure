package vdo

import (
	"context"

	"github.com/appliedres/cloudy/logging"
	cm "github.com/appliedres/cloudy/models"
)

// Creates a Windows VM
func (vdo *VirtualDesktopOrchestrator) createWindowsVM(ctx context.Context, vm *cm.VirtualMachine, apiKeySecret, agentBinaryURL *string) (*cm.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "Creating Windows VM")

	vm, err := vdo.vmManager.CreateVirtualMachine(ctx, vm)
	if err != nil {
		return nil, logging.LogMsgAndErr(ctx, log, err, "CreateVirtualMachine failed creating Windows VM")
	}

	log.DebugContext(ctx, "Running initial setup for Windows VM")
	vm, err = vdo.virtualMachineSetupWindows(ctx, vm, apiKeySecret, agentBinaryURL)
	if err != nil {
		return nil, logging.LogMsgAndErr(ctx, log, err, "CreateVirtualMachine failed during Windows VM setup")
	}

	return vm, err
}
