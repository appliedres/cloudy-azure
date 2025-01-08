package avd

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/logging"
	"github.com/google/uuid"
)

// CreateApplicationGroup creates an application group for the given host pool.
func (avd *AzureVirtualDesktopManager) CreateApplicationGroup(ctx context.Context, rgName, suffix string, tags map[string]*string) (*armdesktopvirtualization.ApplicationGroup, error) {
	appGroupName := appGroupNamePrefix + suffix
	hostPoolName := hostPoolNamePrefix + suffix

	hostPoolArmPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DesktopVirtualization/hostPools/%s",
		avd.credentials.SubscriptionID, rgName, hostPoolName)

	appGroup := armdesktopvirtualization.ApplicationGroup{
		Location: to.Ptr(string(avd.config.Region)),
		Tags:     tags,
		Properties: &armdesktopvirtualization.ApplicationGroupProperties{
			ApplicationGroupType: to.Ptr(armdesktopvirtualization.ApplicationGroupTypeDesktop),
			FriendlyName:         to.Ptr("App Group " + suffix),
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

func (avd *AzureVirtualDesktopManager) AssignGroupToDesktopAppGroup(ctx context.Context, desktopAppGroupName string) error {
	// Source: https://learn.microsoft.com/en-us/answers/questions/2104093/azure-virtual-desktop-application-group-assignment
	scope := fmt.Sprintf("/subscriptions/%s/resourcegroups/%s/providers/Microsoft.DesktopVirtualization/applicationgroups/%s",
		avd.credentials.SubscriptionID, avd.credentials.ResourceGroup, desktopAppGroupName)

	roleDefID := fmt.Sprintf("/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/%s",
		avd.credentials.SubscriptionID, avd.config.DesktopApplicationUserRoleID)

	uuidWithHyphen := uuid.New().String()

	res, err := avd.roleAssignmentsClient.Create(ctx, scope, uuidWithHyphen,
		armauthorization.RoleAssignmentCreateParameters{
			Properties: &armauthorization.RoleAssignmentProperties{
				RoleDefinitionID: to.Ptr(roleDefID),
				PrincipalID:      to.Ptr(string(avd.config.AvdUsersGroupId)),
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
