package vdo

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy/logging"
	cm "github.com/appliedres/cloudy/models"
)

// TODO: lock session host 'slot' during this process so its not factored in for simultaneous requests

func (vdo *VirtualDesktopOrchestrator) LinuxAVDPreCreateSetup(ctx context.Context, vm *cm.VirtualMachine) error {
	log := logging.GetLogger(ctx)

	hostPoolName := vdo.avdManager.Config.PooledHostPoolNamePrefix + vdo.avdManager.Name

	log.InfoContext(ctx, "LinuxAVDPreCreateSetup - Starting", "HostPoolName", hostPoolName)

	maxSessions := int32(2)
	log.DebugContext(ctx, "LinuxAVDPreCreateSetup - MaxSessions", "MaxSessions", maxSessions)

	sessionHosts, err := vdo.avdManager.ListSessionHosts(ctx, hostPoolName)
	if err != nil {
		log.ErrorContext(ctx, "LinuxAVDPreCreateSetup - Failed to list session hosts", "Error", err)
		return fmt.Errorf("failed to list session hosts for host pool %s: %w", hostPoolName, err)
	}
	log.DebugContext(ctx, "LinuxAVDPreCreateSetup - Retrieved session hosts", "Count", len(sessionHosts))

	var bestHost *armdesktopvirtualization.SessionHost
	lowestSessionCount := maxSessions + 1
	log.DebugContext(ctx, "LinuxAVDPreCreateSetup - Initializing lowestSessionCount", "Value", lowestSessionCount)

	for _, sh := range sessionHosts {
		if sh.Properties == nil || sh.Properties.Status == nil {
			log.DebugContext(ctx, "LinuxAVDPreCreateSetup - Skipping session host with nil properties or status")
			continue
		}

		log.DebugContext(ctx, "LinuxAVDPreCreateSetup - Checking session host", "Status", *sh.Properties.Status)
		if *sh.Properties.Status == armdesktopvirtualization.StatusAvailable {
			sessionCount := int32(0)
			if sh.Properties.Sessions != nil {
				sessionCount = *sh.Properties.Sessions
			}
			log.DebugContext(ctx, "LinuxAVDPreCreateSetup - Session count for host", "SessionCount", sessionCount)

			if sessionCount < maxSessions && sessionCount < lowestSessionCount {
				bestHost = sh
				lowestSessionCount = sessionCount
				log.DebugContext(ctx, "LinuxAVDPreCreateSetup - Found better session host", "LowestSessionCount", lowestSessionCount)
			}
		}
	}

	if bestHost == nil {
		log.InfoContext(ctx, "LinuxAVDPreCreateSetup - No suitable session host found, creating a new one")
			
		bestHost, err = vdo.CreateSessionHost(ctx, hostPoolName)
		if err != nil {
			log.ErrorContext(ctx, "LinuxAVDPreCreateSetup - Failed to create new session host", "Error", err)
			return fmt.Errorf("failed to create new session host: %w", err)
		}
	}

	log.InfoContext(ctx, "LinuxAVDPreCreateSetup - Completed successfully")
	return nil
}

// Perform post-creation setup for Linux AVD VMs
// VM must include NIC with private IP
// Steps:
// - Create application group
// - Add application group to workspace
// - Create RDP application
// - Generate RDP URL
// - Assign user to application group
func (vdo *VirtualDesktopOrchestrator) LinuxAVDPostCreateSetup(ctx context.Context, vm cm.VirtualMachine) (*cm.VirtualMachine, error) {
	log := logging.GetLogger(ctx)

	hostPoolName := vdo.avdManager.Config.PooledHostPoolNamePrefix + vdo.avdManager.Name

	log.InfoContext(ctx, "LinuxAVDPostCreateSetup - Starting", "HostPoolName", hostPoolName, "VMID", vm.ID)

	suffix := vm.ID + "-linux-avd"
	tags := map[string]*string{
		"arkloud-created-by": to.Ptr("cloudy-azure: vdo orchestrator - LinuxAVDPostCreateSetup"),
		"vmid":             to.Ptr(vm.ID),
	}
	log.DebugContext(ctx, "LinuxAVDPostCreateSetup - Suffix and Tags", "Suffix", suffix, "Tags", tags)

	appGroup, err := vdo.avdManager.CreatePooledRemoteAppApplicationGroup(ctx, suffix, tags)
	if err != nil {
		log.ErrorContext(ctx, "LinuxAVDPostCreateSetup - Failed to create application group", "Error", err)
		return nil, fmt.Errorf("failed to create application group: %w", err)
	}
	log.DebugContext(ctx, "LinuxAVDPostCreateSetup - Created application group", "AppGroupName", *appGroup.Name)

	err = vdo.avdManager.AddApplicationGroupToWorkspace(ctx, vdo.avdManager.Config.PooledWorkspaceNamePrefix+vdo.avdManager.Name, *appGroup.Name)
	if err != nil {
		log.ErrorContext(ctx, "LinuxAVDPostCreateSetup - Failed to add application group to workspace", "Error", err)
		return nil, fmt.Errorf("failed to add application group to workspace: %w", err)
	}
	log.DebugContext(ctx, "LinuxAVDPostCreateSetup - Added application group to workspace", "WorkspaceName", vdo.avdManager.Config.PooledWorkspaceNamePrefix+vdo.avdManager.Name)

	privateIP, err := getPrivateIPFromNICs(ctx, vm.Nics)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "Failed to get private IP for Linux AVD start steps")
	}

	appName := vm.ID + "-linux-avd"
	rdpApp, err := vdo.avdManager.CreateRDPApplication(ctx, *appGroup.Name, appName, *privateIP)
	if err != nil {
		log.ErrorContext(ctx, "LinuxAVDPostCreateSetup - Failed to create RDP application", "Error", err)
		return nil, fmt.Errorf("failed to create RDP application: %w", err)
	}
	log.DebugContext(ctx, "LinuxAVDPostCreateSetup - Created RDP application", "AppName", appName)

	workspaceName := vdo.avdManager.Config.PooledWorkspaceNamePrefix + vdo.avdManager.Name
	workspace, err := vdo.avdManager.GetWorkspaceByName(ctx, workspaceName)
	if err != nil {
		log.ErrorContext(ctx, "LinuxAVDPostCreateSetup - Failed to get workspace ID", "Error", err)
		return nil, fmt.Errorf("failed to get workspace ID: %w", err)
	}	

	workspaceID := *workspace.Properties.ObjectID
	resourceID := *rdpApp.Properties.ObjectID
	url := vdo.avdManager.GenerateWindowsClientURI(workspaceID, resourceID, vm.UserID, vdo.avdManager.Config.UriEnv, vdo.avdManager.Config.UriVersion, false)
	log.DebugContext(ctx, "LinuxAVDPostCreateSetup - Generated RDP URL", "URL", url)

	connection := &cm.VirtualMachineConnection{
		RemoteDesktopProvider: "AVD",
		URL:                   url,
	}
	vm.Connect = connection

	vdo.avdManager.AssignAVDUserGroupToAppGroup(ctx, *appGroup.Name)  // FIXME: this should be limited to the user only, not group
	// vdo.avdManager.AssignPrincipalToAppGroup(ctx, *appGroup.Name, vm.UserID)
	log.DebugContext(ctx, "LinuxAVDPostCreateSetup - Assigned user to app group", "UserID", vm.UserID)

	log.InfoContext(ctx, "LinuxAVDPostCreateSetup - Completed successfully")
	return &vm, nil
}
