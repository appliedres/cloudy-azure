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

// CreateApplicationGroup creates an application group for the given host pool.
func (avd *AzureVirtualDesktopManager) CreateApplicationGroup(ctx context.Context, rgName, suffix string, tags map[string]*string) (*armdesktopvirtualization.ApplicationGroup, error) {
	appGroupName := avd.Config.AppGroupNamePrefix + suffix
	hostPoolName := avd.Config.HostPoolNamePrefix + suffix

	hostPoolArmPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DesktopVirtualization/hostPools/%s",
		avd.Credentials.SubscriptionID, rgName, hostPoolName)

	appGroup := armdesktopvirtualization.ApplicationGroup{
		Location: to.Ptr(string(avd.Credentials.Region)),
		Tags:     tags,
		Properties: &armdesktopvirtualization.ApplicationGroupProperties{
			ApplicationGroupType: to.Ptr(armdesktopvirtualization.ApplicationGroupTypeDesktop),
			FriendlyName:         to.Ptr("App Group for AVD stack '" + suffix + "'"),
			Description:          to.Ptr("Generated via cloudy-azure"),
			HostPoolArmPath:      to.Ptr(hostPoolArmPath),
		},
	}

	resp, err := avd.appGroupsClient.CreateOrUpdate(ctx, rgName, appGroupName, appGroup, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create application group: %w", err)
	}
	return &resp.ApplicationGroup, nil
}

// Given a Host Pool, finds the Desktop Application Group linked to it
func (avd *AzureVirtualDesktopManager) GetDesktopApplicationGroupFromHostpool(ctx context.Context, rgName string, hpName string) (*armdesktopvirtualization.ApplicationGroup, error) {
	log := logging.GetLogger(ctx)

	pager := avd.appGroupsClient.NewListByResourceGroupPager(rgName, nil)
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

func (avd *AzureVirtualDesktopManager) GetDesktopAppGroupByName(ctx context.Context, rgName string, desktopAppGroup string) (*armdesktopvirtualization.ApplicationGroup, error) {
	log := logging.GetLogger(ctx)

	appGroup, err := avd.GetAppGroupByName(ctx, rgName, desktopAppGroup)
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

// GetAppGroupByName retrieves an application group by its name.
// If the application group is not found, it returns nil.
func (avd *AzureVirtualDesktopManager) GetAppGroupByName(ctx context.Context, rgName string, appGroupName string) (*armdesktopvirtualization.ApplicationGroup, error) {
	log := logging.GetLogger(ctx)

	resp, err := avd.appGroupsClient.Get(ctx, rgName, appGroupName, nil)
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
		return nil, fmt.Errorf("application group with name %s not found in resource group %s", appGroupName, rgName)
	}

	log.DebugContext(ctx, "Application group found", "App Group name", *appGroup.Name)
	return &appGroup, nil
}

func (avd *AzureVirtualDesktopManager) AssignRoleToAppGroup(ctx context.Context, appGroupName string) error {
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
				PrincipalID:      to.Ptr(string(avd.Config.AvdUsersGroupId)),
			},
		}, nil)

	if err != nil {
		return cloudy.Error(ctx, "AssignRoleToGroup failure: %+v", err)
	}
	if res.ID == nil {
		return cloudy.Error(ctx, "AssignRoleToGroup failure: role ID is empty")
	}

	_ = res
	return nil
}
