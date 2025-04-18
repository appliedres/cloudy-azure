package vdo

import (
	"context"

	"github.com/appliedres/cloudy/logging"
	cm "github.com/appliedres/cloudy/models"
)

// Creates a Windows VM
func (vdo *VirtualDesktopOrchestrator) createWindowsVM(ctx context.Context, vm *cm.VirtualMachine) (*cm.VirtualMachine, error) {
    log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "Creating Windows VM")

	vm, err := vdo.vmManager.CreateVirtualMachine(ctx, vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "CreateVirtualMachine failed creating Windows VM")
	}

	log.DebugContext(ctx, "Running initial setup for Windows VM")
	vm, err = vdo.virtualMachineSetupWindows(ctx, vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "CreateVirtualMachine failed during Windows VM setup")
	}

    return vm, err
}

// Creates a Linux VM with AVD
func (vdo *VirtualDesktopOrchestrator) createLinuxVMWithAVD(ctx context.Context, vm *cm.VirtualMachine) (*cm.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "Creating Linux VM with AVD")

	err := vdo.LinuxAVDPreCreateSetup(ctx, vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "CreateVirtualMachine failed during Linux AVD pre-create setup")
	}

	log.DebugContext(ctx, "Creating Linux VM")
	vm, err = vdo.vmManager.CreateVirtualMachine(ctx, vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "CreateVirtualMachine failed creating Linux VM")
	}

	// run shell script on VM to install salt minion, etc
	// FIXME: re-enable salt minion on linux
	// vm, err = vdo.virtualMachineSetupLinux(ctx, vm)
	// if err != nil {
	// 	return nil, logging.LogAndWrapErr(ctx, log, err, "CreateVirtualMachine failed during Linux VM setup")
	// }

	log.DebugContext(ctx, "Running post-create setup for Linux AVD")
	vm, err = vdo.LinuxAVDPostCreateSetup(ctx, *vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "CreateVirtualMachine failed during Linux AVD post-create setup")
	}

	log.DebugContext(ctx, "Finished creating Linux VM with AVD")
	return vm, err
}

// Creates a basic Linux VM (non-AVD)
func (vdo *VirtualDesktopOrchestrator) createBasicLinuxVM(ctx context.Context, vm *cm.VirtualMachine) (*cm.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "Creating basic Linux VM (non-AVD)")

	vm, err := vdo.vmManager.CreateVirtualMachine(ctx, vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "CreateVirtualMachine failed creating Linux VM")
	}

	// TODO: cleanup/integrate the case statement in the setup method with this case statement
	// FIXME: re-enable salt minion on linux
	// log.DebugContext(ctx, "Running initial setup for Linux VM")
	// vm, err = vdo.virtualMachineSetupLinux(ctx, vm)
	// if err != nil {
	// 	return nil, logging.LogAndWrapErr(ctx, log, err, "CreateVirtualMachine failed during Linux VM setup")
	// }

	log.DebugContext(ctx, "Finished creating basic Linux VM")
	return vm, err
}
