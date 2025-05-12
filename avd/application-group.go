package avd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/logging"
	"github.com/google/uuid"
)

// CreateApplicationGroupBase creates an application group for the given host pool and app group names.
func (avd *AzureVirtualDesktopManager) CreateApplicationGroup(ctx context.Context, appGroupName, hostPoolName string, tags map[string]*string, appGroupType armdesktopvirtualization.ApplicationGroupType) (*armdesktopvirtualization.ApplicationGroup, error) {
	hostPoolArmPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DesktopVirtualization/hostPools/%s",
		avd.Credentials.SubscriptionID, avd.Credentials.ResourceGroup, hostPoolName)

	appGroup := armdesktopvirtualization.ApplicationGroup{
		Location: to.Ptr(string(avd.Credentials.Region)),
		Tags:     tags,
		Properties: &armdesktopvirtualization.ApplicationGroupProperties{
			ApplicationGroupType: to.Ptr(appGroupType),
			FriendlyName:         to.Ptr("App Group for AVD stack '" + appGroupName + "'"),
			Description:          to.Ptr("Generated via cloudy-azure"),
			HostPoolArmPath:      to.Ptr(hostPoolArmPath),
		},
	}

	resp, err := avd.applicationGroupsClient.CreateOrUpdate(ctx, avd.Credentials.ResourceGroup, appGroupName, appGroup, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create application group: %w", err)
	}
	return &resp.ApplicationGroup, nil
}

// creates a pooled application group for the given suffix.
func (avd *AzureVirtualDesktopManager) CreatePooledRemoteAppApplicationGroup(ctx context.Context, vmID string, tags map[string]*string) (*armdesktopvirtualization.ApplicationGroup, error) {
	appGroupName, err := avd.GenerateLinuxAVDAppGroupNameFromVMID(vmID)
	if err != nil {
		return nil, err
	}

	hostPoolName := avd.Config.PooledHostPoolNamePrefix + avd.Name

	return avd.CreateApplicationGroup(ctx, appGroupName, hostPoolName, tags, armdesktopvirtualization.ApplicationGroupTypeRemoteApp)
}

// Given a Host Pool, finds the Desktop Application Group linked to it
func (avd *AzureVirtualDesktopManager) GetDesktopApplicationGroupFromHostpool(ctx context.Context, hpName string) (*armdesktopvirtualization.ApplicationGroup, error) {
	log := logging.GetLogger(ctx)

	pager := avd.applicationGroupsClient.NewListByResourceGroupPager(avd.Credentials.ResourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list application groups: %w", err)
		}
		for _, group := range page.Value {
			if group.Properties != nil && group.Properties.HostPoolArmPath != nil {
				hostPoolPathSegments := strings.Split(*group.Properties.HostPoolArmPath, "/")
				parsedHostPoolName := hostPoolPathSegments[len(hostPoolPathSegments)-1]

				if parsedHostPoolName == hpName && *group.Properties.ApplicationGroupType == armdesktopvirtualization.ApplicationGroupTypeDesktop {
					log.DebugContext(ctx, "Found Desktop Application Group linked to Host Pool", "Desktop App Group Name", *group.Name, "Host Pool Name", hpName)
					return group, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no desktop application group found for host pool %s", hpName)
}

func (avd *AzureVirtualDesktopManager) GetDesktopAppGroupByName(ctx context.Context, desktopAppGroup string) (*armdesktopvirtualization.ApplicationGroup, error) {
	log := logging.GetLogger(ctx)

	appGroup, err := avd.GetAppGroupByName(ctx, desktopAppGroup)
	if err != nil {
		log.DebugContext(ctx, "Error getting application group", "App Group name", desktopAppGroup, "Error", err)
		return nil, fmt.Errorf("failed to get application group: %w", err)
	}

	if appGroup == nil {
		log.DebugContext(ctx, "Application group not found", "App Group name", desktopAppGroup)
		return nil, fmt.Errorf("application group %s not found", desktopAppGroup)
	}

	if appGroup.Properties == nil || appGroup.Properties.ApplicationGroupType == nil {
		log.DebugContext(ctx, "Application group properties are nil", "App Group name", desktopAppGroup)
		return nil, fmt.Errorf("application group properties are nil for %s", desktopAppGroup)
	}

	// Verify application group type is Desktop
	if *appGroup.Properties.ApplicationGroupType != armdesktopvirtualization.ApplicationGroupTypeDesktop {
		log.DebugContext(ctx, "Application group type is not Desktop", "App Group name", desktopAppGroup)
		return nil, fmt.Errorf("application group %s is not of type Desktop", desktopAppGroup)
	}

	log.DebugContext(ctx, "Application group found", "App Group name", *appGroup.Name)
	return appGroup, nil
}

// Retrieves an application group by its name.
// If the application group is not found, it returns nil.
func (avd *AzureVirtualDesktopManager) GetAppGroupByName(ctx context.Context, appGroupName string) (*armdesktopvirtualization.ApplicationGroup, error) {
	log := logging.GetLogger(ctx)

	resp, err := avd.applicationGroupsClient.Get(ctx, avd.Credentials.ResourceGroup, appGroupName, nil)
	if err != nil {
		// Check if it's a "Not Found" error (404):
		var respErr *azcore.ResponseError
		if ok := errors.As(err, &respErr); ok && respErr.StatusCode == 404 {
			log.DebugContext(ctx, "Application group not found", "AppGroupName", appGroupName)
			return nil, nil
		}

		log.DebugContext(ctx, "Error getting application group", "App Group name", appGroupName, "Error", err)
		return nil, fmt.Errorf("failed to get application group: %w", err)
	}

	appGroup := resp.ApplicationGroup

	// Check if the application group name matches the one we are looking for
	if appGroup.Name == nil || *appGroup.Name != appGroupName {
		log.DebugContext(ctx, "Application group name does not match", "App Group name", *appGroup.Name)
		return nil, fmt.Errorf("application group with name %s not found in resource group %s", appGroupName, avd.Credentials.ResourceGroup)
	}

	log.DebugContext(ctx, "Application group found", "App Group name", *appGroup.Name)
	return &appGroup, nil
}

func (avd *AzureVirtualDesktopManager) AssignUserToAppGroup(ctx context.Context, upn, appGroupName string) error {
	log := logging.GetLogger(ctx)

	userObjectID, err := avd.objectIDFromUPN(ctx, upn)
	if err != nil {
		log.ErrorContext(ctx, "objectIDFromUPN failure", "UPN", upn, "Error", err)
		return cloudy.Error(ctx, "objectIDFromUPN failure: %+v", err)
	}
	
	err = avd.AssignPrincipalToAppGroup(ctx, appGroupName, userObjectID)
	if err != nil {
		log.ErrorContext(ctx, "AssignPrincipalToAppGroup failure", "UPN", upn, "AppGroupName", appGroupName, "UserObjectID", userObjectID, "Error", err)
		return cloudy.Error(ctx, "AssignPrincipalToAppGroup failure: %+v", err)
	}

	log.DebugContext(ctx, "Assigned user group to application group", "UPN", upn, "AppGroupName", appGroupName, "UserObjectID", userObjectID)
	return nil
}

func (avd *AzureVirtualDesktopManager) AssignAVDUserGroupToAppGroup(ctx context.Context, appGroupName string) error {
	err := avd.AssignPrincipalToAppGroup(ctx, appGroupName, string(avd.Config.AvdUsersGroupId))
	if err != nil {
		return cloudy.Error(ctx, "AssignPrincipalToAppGroup failure: %+v", err)
	}

	return nil
}

// TODO assign user to app group

// Used for assigning a role to a user or group in an application group
func (avd *AzureVirtualDesktopManager) AssignPrincipalToAppGroup(ctx context.Context, appGroupName string, principalID string) error {
	// Source: https://learn.microsoft.com/en-us/answers/questions/2104093/azure-virtual-desktop-application-group-assignment
	scope := fmt.Sprintf("/subscriptions/%s/resourcegroups/%s/providers/Microsoft.DesktopVirtualization/applicationgroups/%s",
		avd.Credentials.SubscriptionID, avd.Credentials.ResourceGroup, appGroupName)

	roleDefID := fmt.Sprintf("/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/%s",
		avd.Credentials.SubscriptionID, avd.Config.DesktopApplicationUserRoleID)

	uuidWithHyphen := uuid.New().String()

	res, err := avd.roleAssignmentsClient.Create(ctx, scope, uuidWithHyphen,
		armauthorization.RoleAssignmentCreateParameters{
			Properties: &armauthorization.RoleAssignmentProperties{
				RoleDefinitionID: to.Ptr(roleDefID),
				PrincipalID:      to.Ptr(principalID),
			},
		}, nil)

	if err != nil {
		return cloudy.Error(ctx, "AssignRoleToGroup failure: %+v", err)
	}
	if res.ID == nil {
		return cloudy.Error(ctx, "AssignRoleToGroup failure: role ID is empty")
	}

	return nil
}

func (avd *AzureVirtualDesktopManager) RemovePrincipalFromAppGroup(ctx context.Context, appGroupName string, roleAssignmentName string) error {
	scope := fmt.Sprintf("/subscriptions/%s/resourcegroups/%s/providers/Microsoft.DesktopVirtualization/applicationgroups/%s",
		avd.Credentials.SubscriptionID, avd.Credentials.ResourceGroup, appGroupName)

	res, err := avd.roleAssignmentsClient.Delete(ctx, scope, roleAssignmentName, nil)
	if err != nil {
		return cloudy.Error(ctx, "RemovePrincipalFromAppGroup failure: %+v", err)
	}
	if res.ID == nil {
		return cloudy.Error(ctx, "RemovePrincipalFromAppGroup failure: role ID is empty")
	}

	return nil
}

func (avd *AzureVirtualDesktopManager) DeleteApplicationGroup(ctx context.Context, appGroupName string) error {
	log := logging.GetLogger(ctx)
	
	log.DebugContext(ctx, "Deleting application group", "AppGroupName", appGroupName)
	resp, err := avd.applicationGroupsClient.Delete(ctx, avd.Credentials.ResourceGroup, appGroupName, nil)
	if err != nil {
		return fmt.Errorf("failed to delete application group: %w", err)
	}
	_ = resp
	log.DebugContext(ctx, "Deleted application group", "AppGroupName", appGroupName)
	return nil
}

// lists all application groups associated with the specified host pool.
func (avd *AzureVirtualDesktopManager) ListApplicationGroupsForHostPool(ctx context.Context, hostPoolName string) ([]*armdesktopvirtualization.ApplicationGroup, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "Listing application groups for host pool", "HostPoolName", hostPoolName)

	var results []*armdesktopvirtualization.ApplicationGroup

	pager := avd.applicationGroupsClient.NewListByResourceGroupPager(avd.Credentials.ResourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			log.ErrorContext(ctx, "Failed to fetch page of application groups", "Error", err)
			return nil, fmt.Errorf("failed to list application groups: %w", err)
		}

		for _, group := range page.Value {
			if group.Properties != nil && group.Properties.HostPoolArmPath != nil {
				hostPoolPathSegments := strings.Split(*group.Properties.HostPoolArmPath, "/")
				parsedHostPoolName := hostPoolPathSegments[len(hostPoolPathSegments)-1]

				if parsedHostPoolName == hostPoolName {
					log.DebugContext(ctx, "Found matching application group for host pool", "AppGroupName", *group.Name, "Type", string(*group.Properties.ApplicationGroupType))
					results = append(results, group)
				}
			}
		}
	}

	if len(results) == 0 {
		log.DebugContext(ctx, "No application groups found for host pool", "HostPoolName", hostPoolName)
	}

	return results, nil
}

func (avd *AzureVirtualDesktopManager) ListAppGroupAssignments(ctx context.Context, appGroupName string) ([]*armauthorization.RoleAssignment, error) {
	log := logging.GetLogger(ctx)

	scope := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DesktopVirtualization/applicationGroups/%s",
		avd.Credentials.SubscriptionID,
		avd.Credentials.ResourceGroup,
		appGroupName,
	)

	pager := avd.roleAssignmentsClient.NewListForScopePager(scope, &armauthorization.RoleAssignmentsClientListForScopeOptions{
		Filter: nil,
	})

	var assignments []*armauthorization.RoleAssignment

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			log.ErrorContext(ctx, "Error getting next page of assignments", "err", err)
			return nil, err
		}

		assignments = append(assignments, page.Value...)
	}

	return assignments, nil
}

// creates a personal application group for the given suffix.
func (avd *AzureVirtualDesktopManager) CreatePersonalDesktopApplicationGroup(ctx context.Context, suffix string, tags map[string]*string) (*armdesktopvirtualization.ApplicationGroup, error) {
	appGroupName := avd.Config.PersonalAppGroupNamePrefix + suffix
	hostPoolName := avd.Config.PersonalHostPoolNamePrefix + suffix

	return avd.CreateApplicationGroup(ctx, appGroupName, hostPoolName, tags, armdesktopvirtualization.ApplicationGroupTypeDesktop)
}

func (avd *AzureVirtualDesktopManager) GenerateLinuxAVDAppGroupNameFromVMID(vmID string) (string, error) {
	if vmID == "" {
        return "", fmt.Errorf("vmID must not be empty")
    }
	
	return avd.getAppGroupPrefix() + vmID, nil
}

// returns the vmID embedded in the app‑group name,
// or an error if the name doesn’t match the expected prefix.
func (avd *AzureVirtualDesktopManager) ParseVMIDFromLinuxAVDAppGroupName(appGroupName string) (string, error) {
    if !strings.HasPrefix(appGroupName, avd.getAppGroupPrefix()) {
        return "", fmt.Errorf("invalid appGroupName %q: must start with %q", appGroupName, avd.getAppGroupPrefix())
    }
    // everything after the prefix is the VM ID
    return appGroupName[len(avd.getAppGroupPrefix()):], nil
}

func (avd *AzureVirtualDesktopManager) getAppGroupPrefix() string {
	return avd.Config.PooledAppGroupNamePrefix + avd.Name + "-"
}

func (avd *AzureVirtualDesktopManager) objectIDFromUPN(
    ctx context.Context, upn string) (string, error) {

    if _, err := uuid.Parse(upn); err == nil {
        return upn, nil
    }

    usr, err := avd.graphClient.Users().
        ByUserId(upn).                    
        Get(ctx, nil)
    if err != nil {
        return "", fmt.Errorf("graph lookup %q: %w", upn, err)
    }
    return *usr.GetId(), nil
}