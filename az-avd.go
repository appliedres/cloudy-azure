package cloudyazure

import (
	"context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization"
	"github.com/appliedres/cloudy"
	"github.com/google/uuid"
	"github.com/wflentje/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
)

type AzureVirtualDesktopConfig struct {
	AzureCredentials
	subscription string
}

type AzureVirtualDesktop struct {
	config          AzureVirtualDesktopConfig
	workspaces      *armdesktopvirtualization.WorkspacesClient
	hostpools       *armdesktopvirtualization.HostPoolsClient
	sessionhosts    *armdesktopvirtualization.SessionHostsClient
	usersessions    *armdesktopvirtualization.UserSessionsClient
	roleassignments *armauthorization.RoleAssignmentsClient
}

func NewAzureVirtualDesktop(ctx context.Context, config AzureVirtualDesktopConfig) (*AzureVirtualDesktop, error) {
	cred, err := GetAzureClientSecretCredential(config.AzureCredentials)
	if err != nil {
		return nil, cloudy.Error(ctx, "Authentication failure: %+v", err)
	}

	options := arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	}
	workspaces, err := armdesktopvirtualization.NewWorkspacesClient(config.subscription, cred, &options)
	if err != nil {
		return nil, cloudy.Error(ctx, "NewWorkspacesClient failure: %+v", err)
	}

	hostpools, err := armdesktopvirtualization.NewHostPoolsClient(config.subscription, cred, &options)
	if err != nil {
		return nil, cloudy.Error(ctx, "NewHostPoolsClient failure: %+v", err)
	}

	sessionhosts, err := armdesktopvirtualization.NewSessionHostsClient(config.subscription, cred, &options)
	if err != nil {
		return nil, cloudy.Error(ctx, "NewSessionHostsClient failure: %+v", err)
	}

	usersessions, err := armdesktopvirtualization.NewUserSessionsClient(config.subscription, cred, &options)
	if err != nil {
		return nil, cloudy.Error(ctx, "NewUserSessionsClient failure: %+v", err)
	}

	roleassignments, err := armauthorization.NewRoleAssignmentsClient(config.subscription, cred, &options)
	if err != nil {
		return nil, cloudy.Error(ctx, "NewRoleAssignmentsClient failure: %+v", err)
	}

	return &AzureVirtualDesktop{
		config:          config,
		workspaces:      workspaces,
		hostpools:       hostpools,
		sessionhosts:    sessionhosts,
		usersessions:    usersessions,
		roleassignments: roleassignments,
	}, nil
}

func (avd *AzureVirtualDesktop) ListHostPools(ctx context.Context, rg string) ([]*armdesktopvirtualization.HostPool, error) {
	pager := avd.hostpools.NewListByResourceGroupPager(rg, &armdesktopvirtualization.HostPoolsClientListByResourceGroupOptions{})
	var all []*armdesktopvirtualization.HostPool
	for {
		if !pager.More() {
			break
		}
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.HostPoolList.Value...)
	}
	return all, nil
}

func (avd *AzureVirtualDesktop) ListSessionHosts(ctx context.Context, rg string, hostPool string) ([]*armdesktopvirtualization.SessionHost, error) {
	pager := avd.sessionhosts.NewListPager(rg, hostPool, nil)
	var all []*armdesktopvirtualization.SessionHost
	for {
		if !pager.More() {
			break
		}
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Value...)
	}
	return all, nil
}

func (avd *AzureVirtualDesktop) FindFirstAvailableHostPool(ctx context.Context, rg string, upn string) (*armdesktopvirtualization.HostPool, error) {
	// Get all the host pools
	all, err := avd.ListHostPools(ctx, rg)
	if err != nil {
		return nil, err
	}

	for _, hostpool := range all {
		// List all the sessions for a given host pool
		sessions, err := avd.ListSessionHosts(ctx, rg, *hostpool.Name)
		if err != nil {
			return nil, err
		}

		foundUser := false
		for _, session := range sessions {
			assigned := session.Properties.AssignedUser
			if assigned != nil && *assigned == upn {
				foundUser = true
				break
			}
		}

		if !foundUser {
			return hostpool, nil
		}
	}

	return nil, nil
}

func (avd *AzureVirtualDesktop) RetrieveRegistrationToken(ctx context.Context, rg string, hpname string) (*string, error) {

	// avd.hostpools.RetrieveRegistrationToken returns nil if registration token doesn't exist or is expired
	tokenresponse, err := avd.hostpools.RetrieveRegistrationToken(ctx, rg, hpname, nil)

	if tokenresponse.Token == nil {
		// no go function to create/replace a registration key in armdesktopvirtualization
		return nil, cloudy.Error(ctx, "RetrieveRegistrationToken failure: %+v", err)
	}

	return tokenresponse.Token, err
}

func (avd *AzureVirtualDesktop) GetAvailableSessionHost(ctx context.Context, rg string, hpname string) (*string, error) {
	sessions, err := avd.ListSessionHosts(ctx, rg, hpname)
	if err != nil {
		return nil, err
	}

	for _, session := range sessions {
		assigned := session.Properties.AssignedUser
		status := session.Properties.Status
		if assigned == nil && *status == "Available" {
			return session.Name, nil
		}
	}
	return nil, cloudy.Error(ctx, "GetAvailableSessionHost failure (no available session host): %+v", err)
}

func (avd *AzureVirtualDesktop) AssignSessionHost(ctx context.Context, rg string, hpname string, sessionhost string, upn string) error {
	res, err := avd.sessionhosts.Update(ctx, rg, hpname, sessionhost,
		&armdesktopvirtualization.SessionHostsClientUpdateOptions{
			SessionHost: &armdesktopvirtualization.SessionHostPatch{
				Properties: &armdesktopvirtualization.SessionHostPatchProperties{
					AllowNewSession: to.Ptr(true),
					AssignedUser:    to.Ptr(upn),
				},
			},
		})
	if err != nil {
		return cloudy.Error(ctx, "AssignSessionHost failure: %+v", err)
	}

	_ = res

	return nil
}

func (avd *AzureVirtualDesktop) DeleteUserSession(ctx context.Context, rg string, hpname string, sessionHost string, upn string) error {
	sessionId, err := avd.getUserSessionId(ctx, rg, hpname, sessionHost, upn)
	if err != nil {
		return cloudy.Error(ctx, "UnassignSessionHost failure (no user session): %+v", err)
	}

	res, err := avd.usersessions.Delete(ctx, rg, hpname, sessionHost, *sessionId, nil)
	if err != nil {
		return cloudy.Error(ctx, "UnassignSessionHost failure (user session delete failed): %+v", err)
	}
	_ = res

	return nil
}

func (avd *AzureVirtualDesktop) DisconnecteUserSession(ctx context.Context, rg string, hpname string, sessionHost string, upn string) error {
	sessionId, err := avd.getUserSessionId(ctx, rg, hpname, sessionHost, upn)
	if err != nil {
		return cloudy.Error(ctx, "DisconnecteUserSession failure (no user session): %+v", err)
	}

	res, err := avd.usersessions.Disconnect(ctx, rg, hpname, sessionHost, *sessionId, nil)
	if err != nil {
		return cloudy.Error(ctx, "UnassignSessionHost failure (user session disconnect failed ): %+v", err)
	}
	_ = res

	return nil
}

func (avd *AzureVirtualDesktop) AssignRolesToUser(ctx context.Context, rg string, roleid string, upn string) error {
	scope := "/subscriptions/" + avd.config.subscription + "/resourceGroups/" + rg
	roledefid := "/subscriptions/" + avd.config.subscription + "/providers/Microsoft.Authorization/roleDefinitions/" + roleid
	uuidWithHyphen := uuid.New().String()

	res, err := avd.roleassignments.Create(ctx, scope, uuidWithHyphen,
		armauthorization.RoleAssignmentCreateParameters{
			Properties: &armauthorization.RoleAssignmentProperties{
				RoleDefinitionID: to.Ptr(roledefid),
				PrincipalID:      to.Ptr(upn),
			},
		}, &armauthorization.RoleAssignmentsClientCreateOptions{Location: to.Ptr("USGov Virginia")})
	if err != nil && strings.Split(err.Error(), "ERROR CODE: RoleAssignmentExists") == nil {
		return cloudy.Error(ctx, "AssignRolesToUser failure: %+v", err)
	}
	_ = res
	return nil
}

func (avd *AzureVirtualDesktop) getUserSessionId(ctx context.Context, rg string, hpname string, sessionHost string, upn string) (*string, error) {
	pager := avd.usersessions.NewListPager(rg, hpname, sessionHost, nil)
	var all []*armdesktopvirtualization.UserSession
	for {
		if !pager.More() {
			break
		}
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Value...)
	}

	for _, userSession := range all {
		if *userSession.Properties.UserPrincipalName == upn {
			temp := *userSession.Name
			lastInd := strings.LastIndex(temp, "/")
			sessionId := temp[lastInd+1:]
			return &sessionId, nil
		}
	}

	return nil, nil
}
