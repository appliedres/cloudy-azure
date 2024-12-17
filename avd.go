package cloudyazure

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization"
	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/logging"
	cloudymodels "github.com/appliedres/cloudy/models"
	"github.com/google/uuid"
)

type AzureVirtualDesktopManager struct {
	credentials 	*AzureCredentials
	config      	*AzureVirtualDesktopConfig
	
	workspacesClient      *armdesktopvirtualization.WorkspacesClient
	hostPoolsClient       *armdesktopvirtualization.HostPoolsClient
	sessionHostsClient    *armdesktopvirtualization.SessionHostsClient
	userSessionsClient    *armdesktopvirtualization.UserSessionsClient
	roleAssignmentsClient *armauthorization.RoleAssignmentsClient
	appGroupsClient       *armdesktopvirtualization.ApplicationGroupsClient
}

func NewAzureVirtualDesktopManager(ctx context.Context, credentials *AzureCredentials, config *AzureVirtualDesktopConfig) (*AzureVirtualDesktopManager, error) {
	avd := &AzureVirtualDesktopManager{
		credentials: credentials,
		config:      config,
	}
	err := avd.Configure(ctx)
	if err != nil {
		return nil, err
	}

	return avd, nil
}
	
func (avd *AzureVirtualDesktopManager) Configure(ctx context.Context) error {
	cred, err := NewAzureCredentials(avd.credentials)
	if err != nil {
		return err
	}

	// TODO: load host pools list from config
	// TODO: load connection timeout from config

	options := arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	}

	workspacesclient, err := armdesktopvirtualization.NewWorkspacesClient(avd.credentials.SubscriptionID, cred, &options)
	if err != nil {
		return err
	}
	avd.workspacesClient = workspacesclient

	hostPoolsClient, err := armdesktopvirtualization.NewHostPoolsClient(avd.credentials.SubscriptionID, cred, &options)
	if err != nil {
		return err
	}
	avd.hostPoolsClient = hostPoolsClient

	sessionhostsclient, err := armdesktopvirtualization.NewSessionHostsClient(avd.credentials.SubscriptionID, cred, &options)
	if err != nil {
		return err
	}
	avd.sessionHostsClient = sessionhostsclient

	usersessionsclient, err := armdesktopvirtualization.NewUserSessionsClient(avd.credentials.SubscriptionID, cred, &options)
	if err != nil {
		return err
	}
	avd.userSessionsClient = usersessionsclient

	roleassignmentsclient, err := armauthorization.NewRoleAssignmentsClient(avd.credentials.SubscriptionID, cred, &options)
	if err != nil {
		return err
	}
	avd.roleAssignmentsClient = roleassignmentsclient

	appgroupsClient, err := armdesktopvirtualization.NewApplicationGroupsClient(avd.credentials.SubscriptionID, cred, &options)
	if err != nil {
		return err
	}
	avd.appGroupsClient = appgroupsClient

	return nil
}

func (avd *AzureVirtualDesktopManager) FindFirstAvailableHostPool(ctx context.Context, rg string, upn string) (*armdesktopvirtualization.HostPool, error) {
	// Get all the host pools
	all, err := avd.listHostPools(ctx, rg)
	if err != nil {
		return nil, err
	}

	for _, hostpool := range all {
		// List all the sessions for a given host pool
		sessions, err := avd.listSessionHosts(ctx, rg, *hostpool.Name)
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

func (avd *AzureVirtualDesktopManager) RetrieveRegistrationToken(ctx context.Context, rg string, hpname string) (*string, error) {

	// avd.hostpools.RetrieveRegistrationToken returns nil if registration token doesn't exist or is expired
	tokenresponse, err := avd.hostPoolsClient.RetrieveRegistrationToken(ctx, rg, hpname, nil)

	if tokenresponse.Token == nil {
		// no go function to create/replace a registration key in armdesktopvirtualization
		return nil, cloudy.Error(ctx, "RetrieveRegistrationToken failure: %+v", err)
	}

	return tokenresponse.Token, err
}

// Assigns a User to a session host
func (avd *AzureVirtualDesktopManager) AssignSessionHost(ctx context.Context, rgName string, hpName string, shName string, userobjectid string) error {
	res, err := avd.sessionHostsClient.Update(ctx, rgName, hpName, shName,
		&armdesktopvirtualization.SessionHostsClientUpdateOptions{
			SessionHost: &armdesktopvirtualization.SessionHostPatch{
				Properties: &armdesktopvirtualization.SessionHostPatchProperties{
					AllowNewSession: to.Ptr(true),
					AssignedUser:    to.Ptr(userobjectid),
				},
			},
		})

	if err != nil {
		return cloudy.Error(ctx, "AssignSessionHost failure: %+v", err)
	}
	_ = res

	return nil
}

func (avd *AzureVirtualDesktopManager) DeleteSessionHost(ctx context.Context, rg string, hpname string, sessionhost string) error {
	// removes session host from host pool, does not delete VM

	res, err := avd.sessionHostsClient.Delete(ctx, rg, hpname, sessionhost, nil)
	if err != nil {
		return cloudy.Error(ctx, "AssignSessionHost failure: %+v", err)
	}
	_ = res

	return nil
}

func (avd *AzureVirtualDesktopManager) DeleteUserSession(ctx context.Context, rg string, hpname string, sessionHost string, upn string) error {
	sessionId, err := avd.getUserSessionId(ctx, rg, hpname, sessionHost, upn)
	if err != nil {
		return cloudy.Error(ctx, "UnassignSessionHost failure (no user session): %+v", err)
	}

	res, err := avd.userSessionsClient.Delete(ctx, rg, hpname, sessionHost, *sessionId, nil)
	if err != nil {
		return cloudy.Error(ctx, "UnassignSessionHost failure (user session delete failed): %+v", err)
	}
	_ = res

	return nil
}

func (avd *AzureVirtualDesktopManager) DisconnecteUserSession(ctx context.Context, rg string, hpname string, sessionHost string, upn string) error {
	sessionId, err := avd.getUserSessionId(ctx, rg, hpname, sessionHost, upn)
	if err != nil {
		return cloudy.Error(ctx, "DisconnecteUserSession failure (no user session): %+v", err)
	}

	res, err := avd.userSessionsClient.Disconnect(ctx, rg, hpname, sessionHost, *sessionId, nil)
	if err != nil {
		return cloudy.Error(ctx, "UnassignSessionHost failure (user session disconnect failed ): %+v", err)
	}
	_ = res

	return nil
}

func (avd *AzureVirtualDesktopManager) AssignRoleToUser(ctx context.Context, rg string, roleid string, upn string) error {
	scope := "/subscriptions/" + avd.credentials.SubscriptionID + "/resourceGroups/" + rg
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

func (avd *AzureVirtualDesktopManager) getUserSessionId(ctx context.Context, rg string, hpname string, sessionHost string, upn string) (*string, error) {
	pager := avd.userSessionsClient.NewListPager(rg, hpname, sessionHost, nil)
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

func (avd *AzureVirtualDesktopManager) listHostPools(ctx context.Context, rg string) ([]*armdesktopvirtualization.HostPool, error) {
	// TODO: add back resource group 
	pager := avd.hostPoolsClient.NewListPager(&armdesktopvirtualization.HostPoolsClientListOptions{})
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

func (avd *AzureVirtualDesktopManager) listSessionHosts(ctx context.Context, rg string, hostPool string) ([]*armdesktopvirtualization.SessionHost, error) {
	pager := avd.sessionHostsClient.NewListPager(rg, hostPool, nil)
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

// only used if there is a pool of available VMs to assign to users
func (avd *AzureVirtualDesktopManager) getAvailableSessionHost(ctx context.Context, rg string, hpname string) (*string, error) {
	sessions, err := avd.listSessionHosts(ctx, rg, hpname)
	if err != nil {
		return nil, err
	}

	for _, session := range sessions {
		assigned := session.Properties.AssignedUser
		status := session.Properties.Status
		if assigned == nil && *status == "Available" {
			temp := *session.Name
			lastInd := strings.LastIndex(temp, "/")
			if lastInd == -1 {
				return session.Name, nil
			}
			sessionName := temp[lastInd+1:]
			return &sessionName, nil
		}
	}
	return nil, cloudy.Error(ctx, "GetAvailableSessionHost failure (no available session host): %+v", err)
}

// Prior to VM registration, this process generates a token for a given host pool.
// This token will later be used in the registration process to join the VM to the host pool
// The user is also assigned to the related desktop application group.
func (avd *AzureVirtualDesktopManager) PreRegister(ctx context.Context, rgName string, hpName string) (token *string, err error) {
	token, err = avd.RetrieveRegistrationToken(ctx, rgName, hpName)
	if err != nil {
		return nil, err
	}

	// TODO: For now, desktop application group (DAG) membership is handled by adding a user to the "AVDUsers" group, 
	// 	which is manually assigned to the DAG.
	// dag, err := avd.GetDesktopApplicationGroup(ctx, rgName, hpName)
	// if err != nil {
	// 	return nil, nil, err
	// }

	// err = avd.AddUserToApplicationGroup(ctx, rgName, *dag.ID, upn)
	// if err != nil {
	// 	return nil, nil, err
	// }

	return token, nil
}

// After registering, the user must then be assigned to the new session host
func (avd *AzureVirtualDesktopManager) PostRegister(ctx context.Context, rg, hp, vmID, upn string) error {
	sessionHost, err := avd.FindSessionHostForVM(ctx, rg, hp, vmID)
	if err != nil {
		return err
	}
	if sessionHost == nil {
		return fmt.Errorf("Could not find a session host for VM [%s]", vmID)
	}

	// sessionHost.Name is in the format "hostpoolName/sessionHostName", so we need to split it
	parts := strings.SplitN(*sessionHost.Name, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("Could not split sessionHost.Name: %s", *sessionHost.Name)
	}
	sessionHostName := parts[1]
	
	err = avd.AssignSessionHost(ctx, rg, hp, sessionHostName, upn)
	if err != nil {
		return err
	}

	return nil
}

// GenerateConnectionURL generates the connection URL for the VM in a specified host pool
func (avd *AzureVirtualDesktopManager) GenerateConnectionURL(ctx context.Context, hostPoolName string, sessionHostName string, vmID string) (*cloudymodels.VirtualMachineConnection, error) {
	tenantID := avd.credentials.TenantID

	// Validate required inputs
	if tenantID == "" || hostPoolName == "" || sessionHostName == "" {
		return nil, fmt.Errorf("tenantID, hostPoolName, and sessionHostName cannot be empty")
	}

	// Base URL for Azure Virtual Desktop web client
	baseURL := "https://rdweb.wvd.microsoft.com/arm/webclient/index.html"

	// Build query parameters
	queryParams := url.Values{}
	queryParams.Set("tenantId", tenantID)
	queryParams.Set("hostPoolName", hostPoolName)
	queryParams.Set("sessionHostName", sessionHostName)

	// Combine base URL and query parameters
	fullURL := fmt.Sprintf("%s?%s", baseURL, queryParams.Encode())
	
	connection := &cloudymodels.VirtualMachineConnection{
		RemoteDesktopProvider: "AVD",
		URL:                   fullURL,
		VMID:                  vmID,
	}

	return connection, nil
}

// Given a Host Pool, finds the Desktop Application Group linked to it
func (avd *AzureVirtualDesktopManager) GetDesktopApplicationGroup(ctx context.Context, rg string, hp string) (*armdesktopvirtualization.ApplicationGroup, error) {
	log := logging.GetLogger(ctx)
	
	pager := avd.appGroupsClient.NewListByResourceGroupPager(rg, nil)
    for pager.More() {
        page, err := pager.NextPage(ctx)
        if err != nil {
            return nil, fmt.Errorf("failed to list application groups: %w", err)
        }
        for _, group := range page.Value {
            if *group.Properties.HostPoolArmPath == hp && *group.Properties.ApplicationGroupType == armdesktopvirtualization.ApplicationGroupTypeDesktop {
				log.InfoContext(ctx, "Found Desktop Application Group [%s], linked to Host Pool [%s]", group.Name, hp)
				return group, nil
            }
        }
    }

    return nil, fmt.Errorf("no desktop application group found for host pool %s", hp)
}

// Searches for a session host with a name that contains the VMs ID
func (avd *AzureVirtualDesktopManager) FindSessionHostForVM(ctx context.Context, rg string, hostPoolName string, vmName string) (*armdesktopvirtualization.SessionHost, error) {
	allSessionHosts, err := avd.listSessionHosts(ctx, rg, hostPoolName)
	if err != nil {
		return nil, fmt.Errorf("failed to list session hosts: %w", err)
	}

	for _, sessionHost := range allSessionHosts {
		if sessionHost.Properties != nil && sessionHost.Properties.ResourceID != nil {
			if strings.Contains(*sessionHost.Properties.ResourceID, vmName) {
				return sessionHost, nil
			}
		}
	}

	return nil, nil
}

// func (avd *AzureVirtualDesktopManager) AddUserToApplicationGroup(ctx context.Context, rg string, appGroupID string, upn string) error {
//     assignmentName := fmt.Sprintf("%s-assignment", upn)

//     parameters := armdesktopvirtualization.ApplicationGroupAssignment{
//         Properties: &armdesktopvirtualization.ApplicationGroupAssignmentProperties{
//             ApplicationGroupName: &appGroupID,
//             ResourceType:         armdesktopvirtualization.ResourceTypeUser,
//             ResourceID:           &upn,
//         },
//     }

//     _, err := avd.appGroupsClient.CreateOrUpdate(ctx, rg, appGroupID, assignmentName, parameters, nil)
//     if err != nil {
//         return fmt.Errorf("failed to add user %s to application group %s: %w", upn, appGroupID, err)
//     }

//     return nil
// }
