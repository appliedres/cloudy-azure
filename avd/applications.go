package avd

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy/logging"
)

func (avd *AzureVirtualDesktopManager) CreateRDPApplication(ctx context.Context, applicationGroupName, appName, targetIP string) (*armdesktopvirtualization.Application, error) {
	// reference: https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/mstsc
	
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "Creating RDP application", "Application Group Name", applicationGroupName, "App Name", appName)

	rdpExePath := "C:\\Windows\\System32\\mstsc.exe"
	exeArgs := "/v:" + targetIP
	
	application := armdesktopvirtualization.Application{
		Name:     to.Ptr(appName),
		Type:     to.Ptr("Microsoft.DesktopVirtualization/applications"),
		Properties: &armdesktopvirtualization.ApplicationProperties{
			CommandLineSetting: to.Ptr(armdesktopvirtualization.CommandLineSettingRequire),
			CommandLineArguments: to.Ptr(exeArgs),
			// IconPath: to.Ptr(rdpExePath),  TODO: rdp app icon path
			FriendlyName: to.Ptr(appName), // TODO: make display name UVM name
			Description: to.Ptr("an RDP application"),  
			FilePath: to.Ptr(rdpExePath),
			ShowInPortal: to.Ptr(true),  // TODO: enable or not?
		},
	}

	appResp, err := avd.applicationsClient.CreateOrUpdate(ctx, avd.Credentials.ResourceGroup, applicationGroupName, appName, application, nil)
	if err != nil {
		return nil, err
	}

	log.DebugContext(ctx, "Finished creating RDP application", "Application Group Name", applicationGroupName, "App Name", appName)
	return &appResp.Application, nil
}