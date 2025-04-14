package avd

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy/logging"
)

// CreateWorkspace creates a new workspace for the given host pool.
func (avd *AzureVirtualDesktopManager) CreateWorkspace(ctx context.Context, rgName, suffix, appGroupName string, tags map[string]*string) (*armdesktopvirtualization.Workspace, error) {
	workspaceName := avd.Config.WorkspaceNamePrefix + suffix

	appGroupPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DesktopVirtualization/applicationgroups/%s",
		avd.Credentials.SubscriptionID, rgName, appGroupName)

	appGroups := []*string{
		&appGroupPath,
	}

	newWorkspace := armdesktopvirtualization.Workspace{
		Location: to.Ptr(string(avd.Credentials.Region)),
		Tags:     tags,
		Properties: &armdesktopvirtualization.WorkspaceProperties{
			ApplicationGroupReferences: appGroups,
			FriendlyName:               to.Ptr("Workspace for AVD stack '" + suffix + "'"),
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

	resp, err := avd.workspacesClient.Get(ctx, rgName, workspaceName, nil)
	if err != nil {
        // Check if it's a "Not Found" error (404):
        var respErr *azcore.ResponseError
        if ok := errors.As(err, &respErr); ok && respErr.StatusCode == 404 {
            log.DebugContext(ctx, "Workspace not found", "AppGroupName", workspaceName)
            return nil, nil
        }

		log.DebugContext(ctx, "Error getting workspace", "Workspace name", workspaceName, "Error", err)
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	workspace := resp.Workspace

	// Check if the workspace name matches the one we are looking for
	if workspace.Name == nil || *workspace.Name != workspaceName {
		log.DebugContext(ctx, "Workspace name does not match", "Workspace name", *workspace.Name)
		return nil, fmt.Errorf("workspace with name %s not found in resource group %s", workspaceName, rgName)	
	}

	log.DebugContext(ctx, "Workspace found", "Workspace name", *workspace.Name)
	return &workspace, nil
}
