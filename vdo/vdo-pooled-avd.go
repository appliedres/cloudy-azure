package vdo

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/appliedres/cloudy/logging"
	cm "github.com/appliedres/cloudy/models"
)

// VM must include NIC with private IP
func (vdo *VirtualDesktopOrchestrator) linuxAVDPostCreation(ctx context.Context, vm cm.VirtualMachine) (*cm.VirtualMachine, error) {
	log := logging.GetLogger(ctx)
	hostPoolName := vdo.avdManager.Config.PooledHostPoolNamePrefix + vdo.avdManager.Name
	log.InfoContext(ctx, "LinuxAVDPostCreateSetup - Starting", "HostPoolName", hostPoolName, "VMID", vm.ID)

	tags := map[string]*string{
		"arkloud-created-by": to.Ptr("cloudy-azure: vdo orchestrator - LinuxAVDPostCreateSetup"),
		"vmid":               to.Ptr(vm.ID),
	}

	// App Group
	appGroup, err := vdo.avdManager.CreatePooledRemoteAppApplicationGroup(ctx, vm.ID, tags)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "create application group")
	}

	err = vdo.avdManager.AddApplicationGroupToWorkspace(
		ctx, vdo.avdManager.Config.PooledWorkspaceNamePrefix+vdo.avdManager.Name, *appGroup.Name)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "add AG to workspace")
	}

	// RDP Remote App
	privateIP, err := getPrivateIPFromNICs(ctx, vm.Nics)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "get private IP")
	}
	appName := vm.ID + "-linux-avd"
	rdpApp, err := vdo.avdManager.CreateRDPApplication(ctx, *appGroup.Name, appName, *privateIP)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "create RDP app")
	}

	// Generate connection URI
	workspaceName := vdo.avdManager.Config.PooledWorkspaceNamePrefix + vdo.avdManager.Name
	workspace, err := vdo.avdManager.GetWorkspaceByName(ctx, workspaceName)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "get workspace")
	}

	url := vdo.avdManager.GenerateWindowsClientURI(
		*workspace.Properties.ObjectID,
		*rdpApp.Properties.ObjectID,
		vm.UserID,
		vdo.avdManager.Config.UriEnv,
		vdo.avdManager.Config.UriVersion,
		false,
	)
	vm.Connect = &cm.VirtualMachineConnection{RemoteDesktopProvider: "AVD", URL: url}

	// Assign just the VM user to the app group
	vdo.avdManager.AssignUserToAppGroup(ctx, vm.UserID, *appGroup.Name)

	log.InfoContext(ctx, "LinuxAVDPostCreateSetup - Completed successfully")
	return &vm, nil
}
