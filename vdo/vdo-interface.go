package vdo

import (
	"context"

	"github.com/appliedres/cloudy/models"

)

func (vdo *VirtualDesktopOrchestrator) CreateVirtualMachine(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	vm, err := vdo.vmManager.CreateVirtualMachine(ctx, vm)
	if err != nil {
		return nil, err
	}

	vm, err = vdo.InitialVirtualMachineSetup(ctx, vm)
	if err != nil {
		return nil, err
	}

	return vm, nil
}

func (vdo *VirtualDesktopOrchestrator) StartVirtualMachine(ctx context.Context, vmName string) error {
	// TODO: any AVD logic for starting vm - IP address change since we deallocate? - relevant for linux avd
	return vdo.vmManager.StartVirtualMachine(ctx, vmName)
}

func (vdo *VirtualDesktopOrchestrator) StopVirtualMachine(ctx context.Context, vmName string) error {
	// TODO: any AVD logic for stopping vm -  IP address change since we deallocate? - relevant for linux avd
	return vdo.vmManager.StopVirtualMachine(ctx, vmName)
}

func (vdo *VirtualDesktopOrchestrator) DeleteVirtualMachine(ctx context.Context, vmName string) error {
	err := vdo.vmManager.DeleteVirtualMachine(ctx, vmName)
	if err != nil {
		return err
	}
	
	err = vdo.avdManager.Cleanup(ctx, vmName)
	if err != nil {
		return err
	}

	return err
}

func (vdo *VirtualDesktopOrchestrator) GetAllVirtualMachines(ctx context.Context, filter string, attrs []string, includeState bool) (*[]models.VirtualMachine, error) {
	return vdo.vmManager.GetAllVirtualMachines(ctx, filter, attrs, includeState)
}

func (vdo *VirtualDesktopOrchestrator) GetVirtualMachineById(ctx context.Context, id string, includeState bool) (*models.VirtualMachine, error) {
	return vdo.vmManager.GetVirtualMachineById(ctx, id, includeState)
}

func (vdo *VirtualDesktopOrchestrator) UpdateVirtualMachine(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	return vdo.vmManager.UpdateVirtualMachine(ctx, vm)
}
func (vdo *VirtualDesktopOrchestrator) GetAllVirtualMachineSizes(ctx context.Context) (map[string]*models.VirtualMachineSize, error) {
	return vdo.vmManager.GetAllVirtualMachineSizes(ctx)
}
func (vdo *VirtualDesktopOrchestrator) GetVirtualMachineSizesForTemplate(ctx context.Context, template models.VirtualMachineTemplate) (
	matches map[string]*models.VirtualMachineSize,
	worse map[string]*models.VirtualMachineSize,
	better map[string]*models.VirtualMachineSize,
	err error) {
	return vdo.vmManager.GetVirtualMachineSizesForTemplate(ctx, template)
}
func (vdo *VirtualDesktopOrchestrator) GetVirtualMachineSizesWithUsage(ctx context.Context) (map[string]*models.VirtualMachineSize, error) {
	return vdo.vmManager.GetVirtualMachineSizesWithUsage(ctx)
}
func (vdo *VirtualDesktopOrchestrator) GetVirtualMachineUsage(ctx context.Context) (map[string]models.VirtualMachineFamily, error) {
	return vdo.vmManager.GetVirtualMachineUsage(ctx)
}