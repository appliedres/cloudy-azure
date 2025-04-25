package vdo

import (
	"context"

	"github.com/appliedres/cloudy/logging"
	cm "github.com/appliedres/cloudy/models"
)

// Creates a Linux VM with AVD
func (vdo *VirtualDesktopOrchestrator) createLinuxVMWithAVD(ctx context.Context, vm *cm.VirtualMachine) (*cm.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "Creating Linux VM with AVD")

	err := vdo.ensureCapacity(ctx)
	if err != nil {
		return nil, err
	}
	
	// TODO: parallelize some of this

	log.DebugContext(ctx, "Creating Linux VM")
	vm, err = vdo.vmManager.CreateVirtualMachine(ctx, vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "CreateVirtualMachine failed creating Linux VM")
	}

	// run shell script on VM to install salt minion, etc
	vm, err = vdo.virtualMachineSetupLinux(ctx, vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "CreateVirtualMachine failed during Linux VM setup")
	}

	log.DebugContext(ctx, "Running post-create setup for Linux AVD")
	vm, err = vdo.linuxAVDPostCreation(ctx, *vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "CreateVirtualMachine failed during Linux AVD post-create setup")
	}

	log.DebugContext(ctx, "Finished creating Linux VM with AVD")
	return vm, err
}

func (vdo *VirtualDesktopOrchestrator) startLinuxAVD(ctx context.Context, vm *cm.VirtualMachine) error {
	err := vdo.ensureCapacity(ctx)
	if err != nil {
		return err
	}	

	if len(vm.Nics) == 0 {
        if n, e := vdo.vmManager.GetNics(ctx, vm.ID); e == nil { vm.Nics = n } else { return e }
    }
    _, err = vdo.linuxAVDPostCreation(ctx, *vm)
	if err != nil {
		return err
	}

    return err
}

func (vdo VirtualDesktopOrchestrator) stopLinuxAVD(ctx context.Context, vm *cm.VirtualMachine) error {
	return vdo.cleanupLinuxAVD(ctx, vm)
}

func (vdo VirtualDesktopOrchestrator) deleteLinuxAVD(ctx context.Context, vm *cm.VirtualMachine) error {
	return vdo.cleanupLinuxAVD(ctx, vm)
}

// Called for both stop and delete actions
func (vdo *VirtualDesktopOrchestrator) cleanupLinuxAVD(ctx context.Context, vm *cm.VirtualMachine) error {
	log := logging.GetLogger(ctx)

	suffix := vm.ID + "-linux-avd"
	ag := vdo.avdManager.Config.PooledAppGroupNamePrefix + suffix
	ws := vdo.avdManager.Config.PooledWorkspaceNamePrefix + vdo.avdManager.Name

	_ = vdo.avdManager.RemoveApplicationGroupFromWorkspace(ctx, ws, ag)
	_ = vdo.avdManager.DeleteApplicationGroup(ctx, ag)

	err := vdo.ensureCapacity(ctx)
	if err != nil {
		return err
	}

	log.Debug("successfully cleaned up Linux AVD resources")
	return nil
}
