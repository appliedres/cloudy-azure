package vdo

import (
	"context"
	"regexp"

	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
)

const (
	linuxAVDEnabled = true // TODO: config for Linux AVD disable
)

func (vdo *VirtualDesktopOrchestrator) CreateVirtualMachine(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx).With("vmName", vm.Name, "os", vm.Template.OperatingSystem)
	log.InfoContext(ctx, "VM Orchestrator - CreateVirtualMachine starting")
	defer log.InfoContext(ctx, "VM Orchestrator - CreateVirtualMachine complete")

	switch vm.Template.OperatingSystem {
	case models.VirtualMachineTemplateOperatingSystemWindows: 
		return vdo.createWindowsVM(ctx, vm)
	case models.VirtualMachineTemplateOperatingSystemLinuxDeb, models.VirtualMachineTemplateOperatingSystemLinuxRhel:
		if linuxAVDEnabled {
			return vdo.createLinuxVMWithAVD(ctx, vm)
		} else {
			return vdo.createBasicLinuxVM(ctx, vm)
		}
	default:
		return nil, logging.LogAndWrapErr(ctx, log, nil, "CreateVirtualMachine failed: unsupported OS type")
	}
}

func (vdo *VirtualDesktopOrchestrator) StartVirtualMachine(ctx context.Context, vmName string) error {
	log := logging.GetLogger(ctx).With("vmName", vmName)
	log.InfoContext(ctx, "StartVirtualMachine starting")
	defer log.InfoContext(ctx, "StartVirtualMachine complete")

	// Physically start the VM first
	err := vdo.vmManager.StartVirtualMachine(ctx, vmName)
	if err != nil {
		return logging.LogAndWrapErr(ctx, log, err, "StartVirtualMachine failed")
	}

	// Retrieve the VM to check if it’s Linux AVD
	// TODO: pass VM obj as param instead?
	vm, err := vdo.vmManager.GetVirtualMachine(ctx, vmName, false)
	if err != nil {
		return logging.LogAndWrapErr(ctx, log, err,
			"StartVirtualMachine failed to retrieve VM for AVD check")
	}

    switch vm.Template.OperatingSystem {
    case models.VirtualMachineTemplateOperatingSystemLinuxDeb, models.VirtualMachineTemplateOperatingSystemLinuxRhel:
        if linuxAVDEnabled {
            return vdo.startLinuxAVD(ctx, vm)
        }
    }

	return nil
}

func (vdo *VirtualDesktopOrchestrator) StopVirtualMachine(ctx context.Context, vmName string) error {
	log := logging.GetLogger(ctx).With("vmName", vmName)
	log.InfoContext(ctx, "StopVirtualMachine starting")
	defer log.InfoContext(ctx, "StopVirtualMachine complete")

	// First stop the VM
	err := vdo.vmManager.StopVirtualMachine(ctx, vmName)
	if err != nil {
		return logging.LogAndWrapErr(ctx, log, err, "StopVirtualMachine failed to stop VM")
	}

	// Retrieve VM to check if Linux AVD
	vm, err := vdo.vmManager.GetVirtualMachine(ctx, vmName, false)
	if err != nil {
		return logging.LogAndWrapErr(ctx, log, err,
			"StopVirtualMachine failed to retrieve VM for AVD check")
	}

    if linuxAVDEnabled && 
		(vm.Template.OperatingSystem == models.VirtualMachineTemplateOperatingSystemLinuxDeb ||
		vm.Template.OperatingSystem == models.VirtualMachineTemplateOperatingSystemLinuxRhel) {
        return vdo.cleanupLinuxAVD(ctx, vm)

		// TODO: remove the connection info from the VM object? Or save it for auto-start?
	}

	return nil
}

func (vdo *VirtualDesktopOrchestrator) DeleteVirtualMachine(ctx context.Context, vmName string) error {
	log := logging.GetLogger(ctx).With("vmName", vmName)
	log.InfoContext(ctx, "DeleteVirtualMachine starting")
	defer log.InfoContext(ctx, "DeleteVirtualMachine complete")

	vm, err := vdo.vmManager.GetVirtualMachine(ctx, vmName, false)
	if err != nil {
		// If we cannot even retrieve it, proceed with VM Manager deletion but warn
		log.WarnContext(ctx, "Cannot retrieve VM to check OS; will try to delete anyway", "Error", err)
	}

    if linuxAVDEnabled && 
		(vm.Template.OperatingSystem == models.VirtualMachineTemplateOperatingSystemLinuxDeb ||
		vm.Template.OperatingSystem == models.VirtualMachineTemplateOperatingSystemLinuxRhel) {
		_ = vdo.cleanupLinuxAVD(ctx, vm)
    }

	err = vdo.vmManager.DeleteVirtualMachine(ctx, vmName)
	if err != nil {
		return logging.LogAndWrapErr(ctx, log, err, "DeleteVirtualMachine failed during deletion")
	}

	return nil
}

func (vdo *VirtualDesktopOrchestrator) GetAllVirtualMachines(ctx context.Context, attrs []string, includeState bool) (*[]models.VirtualMachine, error) {
	log := logging.GetLogger(ctx).With("includeState", includeState)
	log.InfoContext(ctx, "GetAllVirtualMachines starting")
	defer log.InfoContext(ctx, "GetAllVirtualMachines complete")

	vms, err := vdo.vmManager.GetAllUserVirtualMachines(ctx, attrs, includeState)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "GetAllVirtualMachines failed")
	}

	log.DebugContext(ctx, "Retrieved VMs", "count", len(*vms))
	return vms, nil
}

func (vdo *VirtualDesktopOrchestrator) GetVirtualMachine(ctx context.Context, id string, includeState bool) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx).With("vmId", id, "includeState", includeState)
	log.InfoContext(ctx, "GetVirtualMachine starting")
	defer log.InfoContext(ctx, "GetVirtualMachine complete")

	vm, err := vdo.vmManager.GetVirtualMachine(ctx, id, includeState)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "GetVirtualMachine failed")
	}

	return vm, nil
}

func (vdo *VirtualDesktopOrchestrator) UpdateVirtualMachine(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx).With("vmName", vm.Name)
	log.InfoContext(ctx, "UpdateVirtualMachine starting")
	defer log.InfoContext(ctx, "UpdateVirtualMachine complete")

	updatedVM, err := vdo.vmManager.UpdateVirtualMachine(ctx, vm)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "UpdateVirtualMachine failed")
	}

	return updatedVM, nil
}

func (vdo *VirtualDesktopOrchestrator) GetAllVirtualMachineSizes(ctx context.Context) (map[string]*models.VirtualMachineSize, error) {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "GetAllVirtualMachineSizes starting")
	defer log.InfoContext(ctx, "GetAllVirtualMachineSizes complete")

	sizes, err := vdo.vmManager.GetAllVirtualMachineSizes(ctx)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "GetAllVirtualMachineSizes failed")
	}

	log.DebugContext(ctx, "VM sizes retrieved", "count", len(sizes))
	return sizes, nil
}

func (vdo *VirtualDesktopOrchestrator) GetVirtualMachineSizesForTemplate(ctx context.Context, template models.VirtualMachineTemplate) (
	matches map[string]*models.VirtualMachineSize,
	worse map[string]*models.VirtualMachineSize,
	better map[string]*models.VirtualMachineSize,
	err error) {
	log := logging.GetLogger(ctx).With("templateName", template.Name)
	log.InfoContext(ctx, "GetVirtualMachineSizesForTemplate starting")
	defer log.InfoContext(ctx, "GetVirtualMachineSizesForTemplate complete")

	matches, worse, better, err = vdo.vmManager.GetVirtualMachineSizesForTemplate(ctx, template)
	if err != nil {
		logging.LogAndWrapErr(ctx, log, err, "GetVirtualMachineSizesForTemplate failed")
	}

	return matches, worse, better, err
}

func (vdo *VirtualDesktopOrchestrator) GetVirtualMachineSizesWithUsage(ctx context.Context) (map[string]*models.VirtualMachineSize, error) {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "GetVirtualMachineSizesWithUsage starting")
	defer log.InfoContext(ctx, "GetVirtualMachineSizesWithUsage complete")

	sizes, err := vdo.vmManager.GetVirtualMachineSizesWithUsage(ctx)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "GetVirtualMachineSizesWithUsage failed")
	}

	log.DebugContext(ctx, "VM sizes with usage retrieved", "count", len(sizes))
	return sizes, nil
}

func (vdo *VirtualDesktopOrchestrator) GetVirtualMachineUsage(ctx context.Context) (map[string]models.VirtualMachineFamily, error) {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "GetVirtualMachineUsage starting")
	defer log.InfoContext(ctx, "GetVirtualMachineUsage complete")

	usage, err := vdo.vmManager.GetVirtualMachineUsage(ctx)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "GetVirtualMachineUsage failed")
	}

	log.DebugContext(ctx, "VM usage retrieved", "familyCount", len(usage))
	return usage, nil
}

func getPrivateIPFromNICs(ctx context.Context, vmNICs []*models.VirtualMachineNic) (*string, error) {
	log := logging.GetLogger(ctx)

	// regex for private IP ranges
	privateIPRegex := regexp.MustCompile(`^(10\.\d{1,3}\.\d{1,3}\.\d{1,3}|172\.(1[6-9]|2[0-9]|3[0-1])\.\d{1,3}\.\d{1,3}|192\.168\.\d{1,3}\.\d{1,3})$`)

	for _, nic := range vmNICs {
		if nic.PrivateIP != "" && privateIPRegex.MatchString(nic.PrivateIP) {
			log.DebugContext(ctx, "Found NIC with private IP", "NIC", nic.Name, "PrivateIP", nic.PrivateIP)
			return &nic.PrivateIP, nil
		}
	}

	return nil, logging.LogAndWrapErr(ctx, log, nil, "No valid private IP found in NICs")
}
