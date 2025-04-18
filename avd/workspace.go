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
func (avd *AzureVirtualDesktopManager) CreatePersonalWorkspaceForAppGroup(ctx context.Context, suffix, appGroupName string, tags map[string]*string) (*armdesktopvirtualization.Workspace, error) {
	workspaceName := avd.Config.PersonalWorkspaceNamePrefix + suffix

	appGroupPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DesktopVirtualization/applicationgroups/%s",
		avd.Credentials.SubscriptionID, avd.Credentials.ResourceGroup, appGroupName)

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

	resp, err := avd.workspacesClient.CreateOrUpdate(ctx, avd.Credentials.ResourceGroup, workspaceName, newWorkspace, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}
	return &resp.Workspace, nil
}

func (avd *AzureVirtualDesktopManager) CreatePooledWorkspace(ctx context.Context, suffix string, tags map[string]*string) (*armdesktopvirtualization.Workspace, error) {
	workspaceName := avd.Config.PooledWorkspaceNamePrefix + suffix

	newWorkspace := armdesktopvirtualization.Workspace{
		Location: to.Ptr(string(avd.Credentials.Region)),
		Tags:     tags,
		Properties: &armdesktopvirtualization.WorkspaceProperties{
			FriendlyName: to.Ptr("Workspace for '" + suffix + "'"),
			Description:  to.Ptr("Generated via cloudy-azure"),
		},
	}

	resp, err := avd.workspacesClient.CreateOrUpdate(ctx, avd.Credentials.ResourceGroup, workspaceName, newWorkspace, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}
	return &resp.Workspace, nil
}

func (avd *AzureVirtualDesktopManager) AddApplicationGroupToWorkspace(ctx context.Context, workspaceName, appGroupName string) error {
	log := logging.GetLogger(ctx)

	workspace, err := avd.GetWorkspaceByName(ctx, workspaceName)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	if workspace == nil {
		return fmt.Errorf("workspace %s not found", workspaceName)
	}

	appGroupPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DesktopVirtualization/applicationgroups/%s",
		avd.Credentials.SubscriptionID, avd.Credentials.ResourceGroup, appGroupName)

	workspace.Properties.ApplicationGroupReferences = append(workspace.Properties.ApplicationGroupReferences, &appGroupPath)

	_, err = avd.workspacesClient.CreateOrUpdate(ctx, avd.Credentials.ResourceGroup, workspaceName, *workspace, nil)
	if err != nil {
		return fmt.Errorf("failed to update workspace with application group: %w", err)
	}

	log.DebugContext(ctx, "Added application group to workspace", "Workspace name", workspaceName, "App Group name", appGroupName)
	return nil
}

func (avd *AzureVirtualDesktopManager) RemoveApplicationGroupFromWorkspace(ctx context.Context, workspaceName, appGroupName string) error {
	log := logging.GetLogger(ctx)

	workspace, err := avd.GetWorkspaceByName(ctx, workspaceName)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	if workspace == nil {
		return fmt.Errorf("workspace %s not found", workspaceName)
	}

	appGroupPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DesktopVirtualization/applicationgroups/%s",
		avd.Credentials.SubscriptionID, avd.Credentials.ResourceGroup, appGroupName)

	for i, ref := range workspace.Properties.ApplicationGroupReferences {
		if *ref == appGroupPath {
			workspace.Properties.ApplicationGroupReferences = append(workspace.Properties.ApplicationGroupReferences[:i], workspace.Properties.ApplicationGroupReferences[i+1:]...)
			break
		}
	}

	_, err = avd.workspacesClient.CreateOrUpdate(ctx, avd.Credentials.ResourceGroup, workspaceName, *workspace, nil)
	if err != nil {
		return fmt.Errorf("failed to update workspace with application group: %w", err)
	}

	log.DebugContext(ctx, "Removed application group from workspace", "Workspace name", workspaceName, "App Group name", appGroupName)
	return nil
}

func (avd *AzureVirtualDesktopManager) GetWorkspaceByName(ctx context.Context, workspaceName string) (*armdesktopvirtualization.Workspace, error) {
	log := logging.GetLogger(ctx)

	resp, err := avd.workspacesClient.Get(ctx, avd.Credentials.ResourceGroup, workspaceName, nil)
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
		return nil, fmt.Errorf("workspace with name %s not found in resource group %s", workspaceName, avd.Credentials.ResourceGroup)
	}

	log.DebugContext(ctx, "Workspace found", "Workspace name", *workspace.Name)
	return &workspace, nil
}
