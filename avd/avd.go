package avd

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/appliedres/cloudy"
	"github.com/google/uuid"
)

func (avd *AzureVirtualDesktopManager) AssignRoleToUser(ctx context.Context, rgName string, roleid string, upn string) error {
	scope := "/subscriptions/" + avd.credentials.SubscriptionID + "/resourceGroups/" + rgName
	roledefid := "/subscriptions/" + avd.credentials.SubscriptionID + "/providers/Microsoft.Authorization/roleDefinitions/" + roleid
	uuidWithHyphen := uuid.New().String()

	res, err := avd.roleAssignmentsClient.Create(ctx, scope, uuidWithHyphen,
		armauthorization.RoleAssignmentCreateParameters{
			Properties: &armauthorization.RoleAssignmentProperties{
				RoleDefinitionID: to.Ptr(roledefid),
				PrincipalID:      to.Ptr(upn),
			},
		}, nil)
	if err != nil && strings.Split(err.Error(), "ERROR CODE: RoleAssignmentExists") == nil {
		return cloudy.Error(ctx, "AssignRolesToUser failure: %+v", err)
	}
	_ = res
	return nil
}

// GenerateWindowsClientURI generates a URI for connecting to an AVD session with the Windows client.
func generateWindowsClientURI(workspaceID, resourceID, upn, env, version string, useMultiMon bool) string {
	// https://learn.microsoft.com/en-us/azure/virtual-desktop/uri-scheme
	base := "ms-avd:connect"

	return fmt.Sprintf(
		"%s?workspaceid=%s&resourceid=%s&username=%s&env=%s&version=%s&usemultimon=%t",
		base,
		workspaceID,
		resourceID,
		upn,
		env,
		version,
		useMultiMon,
	)
}

