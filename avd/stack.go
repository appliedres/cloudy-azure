package avd

import (
	"context"
	"fmt"

	"github.com/appliedres/cloudy/logging"
)

// Helper to delete a host pool and its associated resources
func (avd *AzureVirtualDesktopManager) DeleteStack(ctx context.Context, targetHostPoolName string) error {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "Starting deletion of host pool and its associated resources", "hostPoolName", targetHostPoolName)

	if targetHostPoolName == "" {
		return fmt.Errorf("host pool name is empty")
	}

	// Check if the host pool is empty
	log.DebugContext(ctx, "Checking if host pool is empty", "hostPoolName", targetHostPoolName)
	isEmpty, err := avd.IsHostPoolEmpty(ctx, targetHostPoolName)
	if err != nil {
		return err
	}
	if !isEmpty {
		return fmt.Errorf("error deleting host pool and associated resources. Host pool %s is not empty", targetHostPoolName)
	}
	log.DebugContext(ctx, "Host pool is empty", "hostPoolName", targetHostPoolName)

	// Extract the suffix from the host pool name
	log.DebugContext(ctx, "Extracting suffix from host pool name", "hostPoolName", targetHostPoolName)
	suffix, err := avd.ExtractSuffixFromHostPoolName(targetHostPoolName)
	if err != nil {
		return err
	}
	log.DebugContext(ctx, "Extracted suffix from host pool name", "suffix", suffix)

	// Find associated app group
	log.DebugContext(ctx, "Finding associated application group", "suffix", suffix)
	targetAppGroupName, err := avd.FindApplicationGroupBySuffix(ctx, suffix)
	if err != nil {
		return fmt.Errorf("no associated application group found for host pool %s: %w", targetHostPoolName, err)
	}
	log.DebugContext(ctx, "Found associated application group", "appGroupName", targetAppGroupName)

	// Find associated workspace
	log.DebugContext(ctx, "Finding associated workspace", "suffix", suffix)
	targetWorkspaceName := avd.Config.PersonalWorkspaceNamePrefix + suffix
	workspace, err := avd.GetWorkspaceByName(ctx, targetWorkspaceName)
	if err != nil || workspace == nil {
		return fmt.Errorf("no associated workspace found for host pool %s: %w", targetHostPoolName, err)
	}
	log.DebugContext(ctx, "Found associated workspace", "workspaceName", targetWorkspaceName)

	// Delete the application group
	log.DebugContext(ctx, "Deleting application group", "appGroupName", targetAppGroupName)
	_, err = avd.applicationGroupsClient.Delete(ctx, avd.Credentials.ResourceGroup, targetAppGroupName, nil)
	if err != nil {
		return fmt.Errorf("error deleting application group %s: %w", targetAppGroupName, err)
	}
	log.DebugContext(ctx, "Deleted application group", "appGroupName", targetAppGroupName)

	// Delete the workspace
	log.DebugContext(ctx, "Deleting workspace", "workspaceName", targetWorkspaceName)
	_, err = avd.workspacesClient.Delete(ctx, avd.Credentials.ResourceGroup, targetWorkspaceName, nil)
	if err != nil {
		return fmt.Errorf("error deleting workspace %s: %w", targetWorkspaceName, err)
	}
	log.DebugContext(ctx, "Deleted workspace", "workspaceName", targetWorkspaceName)

	// Delete the host pool
	log.DebugContext(ctx, "Deleting host pool", "hostPoolName", targetHostPoolName)
	_, err = avd.hostPoolsClient.Delete(ctx, avd.Credentials.ResourceGroup, targetHostPoolName, nil)
	if err != nil {
		return fmt.Errorf("error deleting host pool %s: %w", targetHostPoolName, err)
	}
	log.DebugContext(ctx, "Deleted host pool", "hostPoolName", targetHostPoolName)

	log.InfoContext(ctx, "Completed deletion of host pool and its associated resources", "hostPoolName", targetHostPoolName)
	return nil
}
