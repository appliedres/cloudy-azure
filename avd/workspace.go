package avd

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy/logging"
)

// CreateWorkspace creates a new workspace for the given host pool.
func (avd *AzureVirtualDesktopManager) CreateWorkspace(ctx context.Context, rgName, suffix, appGroupName string, tags map[string]*string) (*armdesktopvirtualization.Workspace, error) {
	workspaceName := avd.config.WorkspaceNamePrefix + suffix

	appGroupPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DesktopVirtualization/applicationgroups/%s",
		avd.credentials.SubscriptionID, rgName, appGroupName)

	appGroups := []*string{
		&appGroupPath,
	}

	newWorkspace := armdesktopvirtualization.Workspace{
		Location: to.Ptr(string(avd.config.Region)),
		Tags:     tags,
		Properties: &armdesktopvirtualization.WorkspaceProperties{
			ApplicationGroupReferences: appGroups,
			FriendlyName:               to.Ptr("Workspace " + suffix),
			Description:                to.Ptr("Generated via cloudy-azure"),
		},
	}

	resp, err := avd.workspacesClient.CreateOrUpdate(ctx, rgName, workspaceName, newWorkspace, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}
	return &resp.Workspace, nil
}

func (avd *AzureVirtualDesktopManager) GetWorkspaceByName(ctx context.Context, rgName string, workspaceName string) (*armdesktopvirtualization.Workspace, error) {
	log := logging.GetLogger(ctx)

	// Create the pager to list workspaces
	pager := avd.workspacesClient.NewListByResourceGroupPager(rgName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list workspaces: %w", err)
		}
		for _, workspace := range page.Value {
			if workspace.Name != nil && *workspace.Name == workspaceName {
				log.DebugContext(ctx, "Found object ID for Workspace", "Workspace name", *workspace.Name)
				return workspace, nil
			}
		}
	}

	return nil, fmt.Errorf("workspace with name %s not found in resource group %s", workspaceName, rgName)
}
