package vdo

import (
	"context"

	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
)

func (vdo *VirtualDesktopOrchestrator) CreateVirtualMachine(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "VM Orchestrator - CreateVirtualMachine starting")
	defer log.InfoContext(ctx, "VM Orchestrator - CreateVirtualMachine complete")
	
	vm, err := vdo.vmManager.CreateVirtualMachine(ctx, vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "VM Orchestrator - CreateVirtualMachine failed when creating VM")
	}

	vm, err = vdo.InitialVirtualMachineSetup(ctx, vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "VM Orchestrator - CreateVirtualMachine failed when setting up VM")
	}

	return vm, nil
}

func (vdo *VirtualDesktopOrchestrator) StartVirtualMachine(ctx context.Context, vmName string) error {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "VM Orchestrator - StartVirtualMachine starting")
	defer log.InfoContext(ctx, "VM Orchestrator - StartVirtualMachine complete")

	err := vdo.vmManager.StartVirtualMachine(ctx, vmName)
	if err != nil {
		return logging.LogAndWrapErr(ctx, log, err, "VM Orchestrator - StartVirtualMachine failed")
	}

	return nil
}

func (vdo *VirtualDesktopOrchestrator) StopVirtualMachine(ctx context.Context, vmName string) error {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "VM Orchestrator - StopVirtualMachine starting")
	defer log.InfoContext(ctx, "VM Orchestrator - StopVirtualMachine complete")

	err := vdo.vmManager.StopVirtualMachine(ctx, vmName)
	if err != nil {
		return logging.LogAndWrapErr(ctx, log, err, "VM Orchestrator - StopVirtualMachine failed")
	}

	return nil
}

func (vdo *VirtualDesktopOrchestrator) DeleteVirtualMachine(ctx context.Context, vmName string) error {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "VM Orchestrator - DeleteVirtualMachine starting")
	defer log.InfoContext(ctx, "VM Orchestrator - DeleteVirtualMachine complete")

	err := vdo.vmManager.DeleteVirtualMachine(ctx, vmName)
	if err != nil {
		return logging.LogAndWrapErr(ctx, log, err, "VM Orchestrator - DeleteVirtualMachine failed during deletion")
	}
	
	err = vdo.avdManager.Cleanup(ctx, vmName)
	if err != nil {
		return logging.LogAndWrapErr(ctx, log, err, "VM Orchestrator - DeleteVirtualMachine failed during cleanup")
	}

	return nil
}

func (vdo *VirtualDesktopOrchestrator) GetAllVirtualMachines(ctx context.Context, filter string, attrs []string, includeState bool) (*[]models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "VM Orchestrator - GetAllVirtualMachines starting")
	defer log.InfoContext(ctx, "VM Orchestrator - GetAllVirtualMachines complete")

	vms, err := vdo.vmManager.GetAllVirtualMachines(ctx, filter, attrs, includeState)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "VM Orchestrator - GetAllVirtualMachines failed")
	}

	return vms, nil
}

func (vdo *VirtualDesktopOrchestrator) GetVirtualMachineById(ctx context.Context, id string, includeState bool) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "VM Orchestrator - GetVirtualMachineById starting")
	defer log.InfoContext(ctx, "VM Orchestrator - GetVirtualMachineById complete")

	vm, err := vdo.vmManager.GetVirtualMachineById(ctx, id, includeState)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "VM Orchestrator - GetVirtualMachineById failed")
	}

	return vm, nil
}

func (vdo *VirtualDesktopOrchestrator) UpdateVirtualMachine(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "VM Orchestrator - UpdateVirtualMachine starting")
	defer log.InfoContext(ctx, "VM Orchestrator - UpdateVirtualMachine complete")

	updatedVM, err := vdo.vmManager.UpdateVirtualMachine(ctx, vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "VM Orchestrator - UpdateVirtualMachine failed")
	}

	return updatedVM, nil
}

func (vdo *VirtualDesktopOrchestrator) GetAllVirtualMachineSizes(ctx context.Context) (map[string]*models.VirtualMachineSize, error) {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "VM Orchestrator - GetAllVirtualMachineSizes starting")
	defer log.InfoContext(ctx, "VM Orchestrator - GetAllVirtualMachineSizes complete")

	sizes, err := vdo.vmManager.GetAllVirtualMachineSizes(ctx)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "VM Orchestrator - GetAllVirtualMachineSizes failed")
	}

	return sizes, nil
}

func (vdo *VirtualDesktopOrchestrator) GetVirtualMachineSizesForTemplate(ctx context.Context, template models.VirtualMachineTemplate) (
	matches map[string]*models.VirtualMachineSize,
	worse map[string]*models.VirtualMachineSize,
	better map[string]*models.VirtualMachineSize,
	err error) {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "VM Orchestrator - GetVirtualMachineSizesForTemplate starting")
	defer log.InfoContext(ctx, "VM Orchestrator - GetVirtualMachineSizesForTemplate complete")

	matches, worse, better, err = vdo.vmManager.GetVirtualMachineSizesForTemplate(ctx, template)
	if err != nil {
		logging.LogAndWrapErr(ctx, log, err, "VM Orchestrator - GetVirtualMachineSizesForTemplate failed")
	}

	return matches, worse, better, err
}

func (vdo *VirtualDesktopOrchestrator) GetVirtualMachineSizesWithUsage(ctx context.Context) (map[string]*models.VirtualMachineSize, error) {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "VM Orchestrator - GetVirtualMachineSizesWithUsage starting")
	defer log.InfoContext(ctx, "VM Orchestrator - GetVirtualMachineSizesWithUsage complete")

	sizes, err := vdo.vmManager.GetVirtualMachineSizesWithUsage(ctx)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "VM Orchestrator - GetVirtualMachineSizesWithUsage failed")
	}

	return sizes, nil
}

func (vdo *VirtualDesktopOrchestrator) GetVirtualMachineUsage(ctx context.Context) (map[string]models.VirtualMachineFamily, error) {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "VM Orchestrator - GetVirtualMachineUsage starting")
	defer log.InfoContext(ctx, "VM Orchestrator - GetVirtualMachineUsage complete")

	usage, err := vdo.vmManager.GetVirtualMachineUsage(ctx)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "VM Orchestrator - GetVirtualMachineUsage failed")
	}

	return usage, nil
}