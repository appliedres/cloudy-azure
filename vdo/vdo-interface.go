package vdo

import (
	"context"
	"regexp"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
)

func (vdo *VirtualDesktopOrchestrator) CreateVirtualMachine(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx).With("vmName", vm.Name, "os", vm.Template.OperatingSystem)
	log.InfoContext(ctx, "VM Orchestrator - CreateVirtualMachine starting")
	defer log.InfoContext(ctx, "VM Orchestrator - CreateVirtualMachine complete")

	switch vm.Template.OperatingSystem {
	case "windows":
		return vdo.createWindowsVM(ctx, vm)
	case "linux":
		linuxAVDEnabled := true // TODO: config for Linux AVD disable
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

	nics, err := vdo.vmManager.GetNics(ctx, vm.ID)
	if err != nil {
		return logging.LogAndWrapErr(ctx, log, err,
			"StartVirtualMachine failed to retrieve Nics for AVD check")
	}
	vm.Nics = nics

	// If Linux AVD, do the AVD "On startup" steps
	if vm.Template != nil && vm.Template.OperatingSystem == "Linux" {
		if len(vm.Nics) == 0 {
			log.WarnContext(ctx, "No NICs found for VM – cannot proceed with Linux AVD start steps")
			return nil
		}
		privateIP, err := getPrivateIPFromNICs(ctx, vm.Nics)
		if err != nil {
			return logging.LogAndWrapErr(ctx, log, err, "Failed to get private IP for Linux AVD start steps")
		}

		err = vdo.LinuxAVDPreCreateSetup(ctx, vm)
		if err != nil {
			return logging.LogAndWrapErr(ctx, log, err,
				"Failed to secure session host in StartVirtualMachine (Linux AVD)")
		}

		suffix := vm.ID + "-linux-avd"

		tags := map[string]*string{
			"arkloud-created-by": to.Ptr("cloudy-azure: vdo orchestrator - StartVirtualMachine"),
			"VMID":               to.Ptr(vm.ID),
		}

		appGroup, err := vdo.avdManager.CreatePooledRemoteAppApplicationGroup(ctx, suffix, tags)
		if err != nil {
			return logging.LogAndWrapErr(ctx, log, err, "Failed to create application group on start")
		}

		err = vdo.avdManager.AddApplicationGroupToWorkspace(ctx,
			vdo.avdManager.Config.PooledWorkspaceNamePrefix+vdo.avdManager.Name, *appGroup.Name)
		if err != nil {
			return logging.LogAndWrapErr(ctx, log, err,
				"Failed to add application group to workspace on start")
		}

		appName := vm.ID + "-linux-avd"
		rdpApp, err := vdo.avdManager.CreateRDPApplication(ctx, *appGroup.Name, appName, *privateIP)
		if err != nil {
			return logging.LogAndWrapErr(ctx, log, err, "Failed to create RDP app on start")
		}

		workspaceName := vdo.avdManager.Config.PooledWorkspaceNamePrefix + vdo.avdManager.Name
		workspace, err := vdo.avdManager.GetWorkspaceByName(ctx, workspaceName)
		if err != nil {
			return logging.LogAndWrapErr(ctx, log, err, "Failed to get workspace ID on start")
		}

		workspaceID := *workspace.Properties.ObjectID
		resourceID := *rdpApp.Properties.ObjectID
		url := vdo.avdManager.GenerateWindowsClientURI(
			workspaceID,
			resourceID,
			vm.UserID,
			vdo.avdManager.Config.UriEnv,
			vdo.avdManager.Config.UriVersion,
			false,
		)

		err = vdo.avdManager.AssignAVDUserGroupToAppGroup(ctx, *appGroup.Name)
		if err != nil {
			return logging.LogAndWrapErr(ctx, log, err,
				"Failed to assign AVD user group to app group on start")
		}

		// Optionally store the new connection info back to the VM object
		vm.Connect = &models.VirtualMachineConnection{
			RemoteDesktopProvider: "AVD",
			URL:                   url,
		}
		// TODO: should we return the VM object with the connection info? It may have changed since the start
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

	// If Linux AVD, we must remove this VMs resources
	if vm.Template != nil && vm.Template.OperatingSystem == "Linux" {
		suffix := vm.ID + "-linux-avd"
		appGroupName := vdo.avdManager.Config.PooledAppGroupNamePrefix + suffix

		err = vdo.avdManager.DeleteApplicationGroup(ctx, appGroupName)
		if err != nil {
			log.WarnContext(ctx, "Failed to delete app group on stop", "Error", err)
		}

		workspaceName := vdo.avdManager.Config.PooledWorkspaceNamePrefix + vdo.avdManager.Name
		vdo.avdManager.RemoveApplicationGroupFromWorkspace(ctx, workspaceName, appGroupName)

		// TODO: Scale down session hosts if needed

		// TODO: remove the connection info from the VM object? Or save it for auto-start?
	}

	return nil
}

func (vdo *VirtualDesktopOrchestrator) DeleteVirtualMachine(ctx context.Context, vmName string) error {
	log := logging.GetLogger(ctx).With("vmName", vmName)
	log.InfoContext(ctx, "DeleteVirtualMachine starting")
	defer log.InfoContext(ctx, "DeleteVirtualMachine complete")

	// Retrieve VM so we can see if it’s Linux AVD
	vm, err := vdo.vmManager.GetVirtualMachine(ctx, vmName, false)
	if err != nil {
		// If we cannot even retrieve it, proceed with VM Manager deletion but warn
		log.WarnContext(ctx, "Cannot retrieve VM to check OS; will try to delete anyway", "Error", err)
	}

	if vm != nil && vm.Template != nil && vm.Template.OperatingSystem == "Linux" {
		suffix := vm.ID + "-linux-avd"
		appGroupName := vdo.avdManager.Config.PooledAppGroupNamePrefix + suffix

		err = vdo.avdManager.DeleteApplicationGroup(ctx, appGroupName)
		if err != nil {
			log.WarnContext(ctx, "Failed to delete app group on stop", "Error", err)
		}

		workspaceName := vdo.avdManager.Config.PooledWorkspaceNamePrefix + vdo.avdManager.Name
		vdo.avdManager.RemoveApplicationGroupFromWorkspace(ctx, workspaceName, appGroupName)

		// TODO: Scale down session hosts if needed
	}

	// d) Delete the Linux (or Windows) VM for real
	err = vdo.vmManager.DeleteVirtualMachine(ctx, vmName)
	if err != nil {
		return logging.LogAndWrapErr(ctx, log, err, "DeleteVirtualMachine failed during deletion")
	}

	return nil
}

func (vdo *VirtualDesktopOrchestrator) GetAllVirtualMachines(ctx context.Context, filter string, attrs []string, includeState bool) (*[]models.VirtualMachine, error) {
	log := logging.GetLogger(ctx).With("filter", filter, "includeState", includeState)
	log.InfoContext(ctx, "GetAllVirtualMachines starting")
	defer log.InfoContext(ctx, "GetAllVirtualMachines complete")

	vms, err := vdo.vmManager.GetAllVirtualMachines(ctx, filter, attrs, includeState)
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
