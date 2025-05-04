package vdo

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/logging"
	cm "github.com/appliedres/cloudy/models"
)

func (vdo *VirtualDesktopOrchestrator) CreateSessionHost(ctx context.Context, hostPoolName string) (*armdesktopvirtualization.SessionHost, error) {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "CreateSessionHost - Starting", "HostPoolName", hostPoolName)

	timestampedID := cloudy.GenerateTimestampIDNow()
	sessionHostID := fmt.Sprintf("shvm-%s", timestampedID)
	sessionHostName := fmt.Sprintf("Session Host VM %s", timestampedID)

	// TODO: make session host template configurable
	sessionHostVM := &cm.VirtualMachine{
		ID: 			sessionHostID,
		Name: 			sessionHostName,
		Description: 	"a session host VM for pooled AVD'",
		Template: &cm.VirtualMachineTemplate{
			OperatingSystem:      "windows",
			OsBaseImageID: "marketplace::microsoftwindowsdesktop::windows-11::win11-22h2-ent::latest",
			LocalAdministratorID: "salt",
			Size: &cm.VirtualMachineSize{
				ID: "Standard_D2s_v4",
			},
			SecurityProfile: &cm.VirtualMachineSecurityProfileConfiguration{
				SecurityTypes: cm.VirtualMachineSecurityTypesTrustedLaunch,
			},
		},
		Tags: map[string]*string{
			"arkloud-created-by": to.Ptr("cloudy-azure: vdo orchestrator - CreateSessionHost"),
		},
		UserID: "system",  // TODO: what to use for UserID? Does this need to be a valid entra UPN?
	}

	// TODO: do we need a separate CreateSessionHostVirtualMachine function? vs CreateUserVirtualMachine?

	// Create the session host VM
	sessionHostVM, err := vdo.vmManager.CreateVirtualMachine(ctx, sessionHostVM)
	if err != nil {
		return nil, fmt.Errorf("failed to create new session host: %w", err)
	}

	// retrieve host pool token
	hostPoolToken, err := vdo.avdManager.RetrieveRegistrationToken(ctx, hostPoolName)
	if err != nil {
		return nil, fmt.Errorf("failed to get host pool token: %w", err)
	}

	// build setup script
	vdoConfig := vdo.config
	vdoConfig.SaltMinionInstall = nil // disable salt minion install
	script, err := vdo.buildVirtualMachineSetupScript(ctx, vdoConfig, hostPoolToken)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "Could not build powershell script (AVD enabled)")
	}

	// run the setup script on the session host VM
	err = vdo.vmManager.ExecuteRemotePowershell(ctx, sessionHostVM.ID, script, 20*time.Minute, 15*time.Second)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "Could not run powershell (AVD enabled)")
	}

	// wait for the session host to be registered with the host pool and 'ready'
	log.DebugContext(ctx, "Waiting for session host to be ready", "HostPoolName", hostPoolName, "VM", sessionHostVM.ID)
	sessionHost, err := vdo.avdManager.WaitForSessionHost(ctx, hostPoolName, sessionHostVM.ID, 10*time.Minute)
	if err != nil {
		log.WarnContext(ctx, "Error waiting for session host", "Error", err)
		return nil, logging.LogAndWrapErr(ctx, log, err, "Waiting for session host to be ready")
	}

	return sessionHost, nil
}
