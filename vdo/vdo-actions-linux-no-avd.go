package vdo

import (
	"context"

	"github.com/appliedres/cloudy/logging"
	cm "github.com/appliedres/cloudy/models"
)

// Creates a basic Linux VM (non-AVD)
func (vdo *VirtualDesktopOrchestrator) createBasicLinuxVM(ctx context.Context, vm *cm.VirtualMachine) (*cm.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "Creating basic Linux VM (non-AVD)")

	vm, err := vdo.vmManager.CreateVirtualMachine(ctx, vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "CreateVirtualMachine failed creating Linux VM")
	}

	log.DebugContext(ctx, "Running initial setup for Linux VM")
	vm, err = vdo.virtualMachineSetupLinux(ctx, vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "CreateVirtualMachine failed during Linux VM setup")
	}

	log.DebugContext(ctx, "Finished creating basic Linux VM")
	return vm, err
}
