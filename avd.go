package cloudyazure

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/logging"
	"github.com/google/uuid"
)

func (avd *AzureVirtualDesktopManager) FindFirstAvailableHostPool(ctx context.Context, rgName string, upn string) (*armdesktopvirtualization.HostPool, error) {
	// Get all the host pools
	all, err := avd.listHostPools(ctx, rgName, nil)
	if err != nil {
		return nil, err
	}

	for _, hostpool := range all {
		// List all the sessions for a given host pool
		sessions, err := avd.listSessionHosts(ctx, rgName, *hostpool.Name)
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

func (avd *AzureVirtualDesktopManager) RetrieveRegistrationToken(ctx context.Context, rgName string, hpname string) (*string, error) {

	// avd.hostpools.RetrieveRegistrationToken returns nil if registration token doesn't exist or is expired
	tokenresponse, err := avd.hostPoolsClient.RetrieveRegistrationToken(ctx, rgName, hpname, nil)

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

func (avd *AzureVirtualDesktopManager) DeleteSessionHost(ctx context.Context, rgName string, hpname string, sessionhost string) error {
	// removes session host from host pool, does not delete VM

	res, err := avd.sessionHostsClient.Delete(ctx, rgName, hpname, sessionhost, nil)
	if err != nil {
		return cloudy.Error(ctx, "AssignSessionHost failure: %+v", err)
	}
	_ = res

	return nil
}

func (avd *AzureVirtualDesktopManager) DeleteUserSession(ctx context.Context, rgName string, hpname string, sessionHost string, upn string) error {
	sessionId, err := avd.getUserSessionId(ctx, rgName, hpname, sessionHost, upn)
	if err != nil {
		return cloudy.Error(ctx, "UnassignSessionHost failure (no user session): %+v", err)
	}

	res, err := avd.userSessionsClient.Delete(ctx, rgName, hpname, sessionHost, *sessionId, nil)
	if err != nil {
		return cloudy.Error(ctx, "UnassignSessionHost failure (user session delete failed): %+v", err)
	}
	_ = res

	return nil
}

func (avd *AzureVirtualDesktopManager) DisconnecteUserSession(ctx context.Context, rgName string, hpname string, sessionHost string, upn string) error {
	sessionId, err := avd.getUserSessionId(ctx, rgName, hpname, sessionHost, upn)
	if err != nil {
		return cloudy.Error(ctx, "DisconnecteUserSession failure (no user session): %+v", err)
	}

	res, err := avd.userSessionsClient.Disconnect(ctx, rgName, hpname, sessionHost, *sessionId, nil)
	if err != nil {
		return cloudy.Error(ctx, "UnassignSessionHost failure (user session disconnect failed ): %+v", err)
	}
	_ = res

	return nil
}

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

func (avd *AzureVirtualDesktopManager) AssignGroupToDesktopAppGroup(ctx context.Context, desktopAppGroupName string) error {
	// Source: https://learn.microsoft.com/en-us/answers/questions/2104093/azure-virtual-desktop-application-group-assignment
	scope := fmt.Sprintf("/subscriptions/%s/resourcegroups/%s/providers/Microsoft.DesktopVirtualization/applicationgroups/%s",
		avd.credentials.SubscriptionID, avd.credentials.ResourceGroup, desktopAppGroupName)

	roleDefID := fmt.Sprintf("/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/%s",
		avd.credentials.SubscriptionID, desktopApplicationUserRoleID)
		
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

func (avd *AzureVirtualDesktopManager) getUserSessionId(ctx context.Context, rgName string, hpname string, sessionHost string, upn string) (*string, error) {
	pager := avd.userSessionsClient.NewListPager(rgName, hpname, sessionHost, nil)
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

func (avd *AzureVirtualDesktopManager) listHostPools(ctx context.Context, rgName string, prefixFilter *string) ([]*armdesktopvirtualization.HostPool, error) {
	pager := avd.hostPoolsClient.NewListByResourceGroupPager(rgName, &armdesktopvirtualization.HostPoolsClientListByResourceGroupOptions{})
	var all []*armdesktopvirtualization.HostPool

	for {
		if !pager.More() {
			break
		}
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		// Filter host pools by prefixFilter
		for _, pool := range resp.HostPoolList.Value {
			if prefixFilter == nil || strings.HasPrefix(*pool.Name, *prefixFilter) {
				all = append(all, pool)
			}
		}
	}

	return all, nil
}

func (avd *AzureVirtualDesktopManager) listSessionHosts(ctx context.Context, rgName string, hostPoolName string) ([]*armdesktopvirtualization.SessionHost, error) {
	pager := avd.sessionHostsClient.NewListPager(rgName, hostPoolName, nil)
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
func (avd *AzureVirtualDesktopManager) getAvailableSessionHost(ctx context.Context, rgName string, hpname string) (*string, error) {
	sessions, err := avd.listSessionHosts(ctx, rgName, hpname)
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

// Searches for a session host with a name that contains the VMs ID
func (avd *AzureVirtualDesktopManager) FindSessionHostByVMNameInHostPool(ctx context.Context, rgName string, hostPoolName string, vmID string) (*armdesktopvirtualization.SessionHost, error) {
	log := logging.GetLogger(ctx)

	log.DebugContext(ctx, "Searching for session host in host pool", "Host Pool Name", hostPoolName)

	allSessionHosts, err := avd.listSessionHosts(ctx, rgName, hostPoolName)
	if err != nil {
		return nil, fmt.Errorf("failed to list session hosts: %w", err)
	}

	for _, sessionHost := range allSessionHosts {
		if sessionHost.Properties != nil && sessionHost.Properties.ResourceID != nil {
			if strings.Contains(*sessionHost.Properties.ResourceID, vmID) {
				return sessionHost, nil
			}
		}
	}

	return nil, nil
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

func (avd *AzureVirtualDesktopManager) GetDesktopApplicationObjectIDFromAppGroup(ctx context.Context, rgName string, appGroup *armdesktopvirtualization.ApplicationGroup) (string, error) {
	log := logging.GetLogger(ctx)

	if appGroup == nil || appGroup.Name == nil {
		return "", fmt.Errorf("invalid application group provided")
	}

	appPager := avd.appsClient.NewListPager(rgName, *appGroup.Name, nil)
	for appPager.More() {
		page, err := appPager.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to list applications for application group %s: %w", *appGroup.Name, err)
		}
		for _, app := range page.Value {
			if app.Properties != nil && app.Properties.FriendlyName != nil {
				log.InfoContext(ctx, "Found Desktop Application in App Group", "App Group Name", appGroup.Name)
				return *app.ID, nil
			}
		}
	}

	return "", fmt.Errorf("no desktop application found in application group %s", *appGroup.Name)
}

func (avd *AzureVirtualDesktopManager) GetAllDesktopApplications(ctx context.Context, rgName string) ([]string, error) {
	log := logging.GetLogger(ctx)

	appGroupPager := avd.appGroupsClient.NewListByResourceGroupPager(rgName, nil)
	var appIDs []string

	for appGroupPager.More() {
		page, err := appGroupPager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list application groups in resource group %s: %w", rgName, err)
		}
		for _, appGroup := range page.Value {
			if appGroup.Name != nil {
				log.InfoContext(ctx, "Found Application Group", "group_name", *appGroup.Name)

				appPager := avd.appsClient.NewListPager(rgName, *appGroup.Name, nil)
				for appPager.More() {
					appPage, err := appPager.NextPage(ctx)
					if err != nil {
						return nil, fmt.Errorf("failed to list applications for application group %s: %w", *appGroup.Name, err)
					}
					for _, app := range appPage.Value {
						if app.Properties != nil && app.Properties.FriendlyName != nil {
							log.InfoContext(ctx, "Found Desktop Application [%s] with Object ID [%s]", *app.Properties.FriendlyName, *app.ID)
							appIDs = append(appIDs, *app.ID)
						}
					}
				}
			}
		}
	}

	if len(appIDs) == 0 {
		return nil, fmt.Errorf("no desktop applications found in resource group %s", rgName)
	}

	return appIDs, nil
}

func (avd *AzureVirtualDesktopManager) listDesktops(ctx context.Context, rgName string, appGroupName string) ([]*armdesktopvirtualization.Desktop, error) {
	pager := avd.desktopsClient.NewListPager(rgName, appGroupName, nil)
	var allDesktops []*armdesktopvirtualization.Desktop

	for {
		if !pager.More() {
			break
		}

		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		allDesktops = append(allDesktops, resp.Value...)
	}

	return allDesktops, nil
}

func (avd *AzureVirtualDesktopManager) getSingleDesktop(ctx context.Context, rgName string, appGroupName string) (*armdesktopvirtualization.Desktop, error) {
	desktops, err := avd.listDesktops(ctx, rgName, appGroupName)
	if err != nil {
		return nil, fmt.Errorf("failed to list desktops: %w", err)
	}

	if len(desktops) == 0 {
		return nil, fmt.Errorf("no desktops found for appGroupName: %s", appGroupName)
	}
	if len(desktops) > 1 {
		return nil, fmt.Errorf("multiple desktops found for appGroupName: %s", appGroupName)
	}

	return desktops[0], nil
}

// WaitForSessionHost waits for a VM to appear as a session host in a specified host pool and ensures its status is 'Available'.
func (avd *AzureVirtualDesktopManager) WaitForSessionHost(ctx context.Context, rgName, hpName, vmID string, timeout time.Duration) (*armdesktopvirtualization.SessionHost, error) {
	// Set up a timer for the timeout
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Second) // TODO: switch to exponential backoff
	defer ticker.Stop()

	for {
		// TODO: switch to multiple waits - 1 for session host existing, another for when it's ready.

		// Check if the VM is registered as a session host
		sessionHost, err := avd.FindSessionHostByVMNameInHostPool(ctx, rgName, hpName, vmID)
		if err != nil {
			return nil, fmt.Errorf("error finding session host: %w", err)
		}

		// If the session host is found, check its status
		if sessionHost != nil {
			if sessionHost.Properties != nil && sessionHost.Properties.Status != nil {
				if *sessionHost.Properties.Status == armdesktopvirtualization.StatusAvailable {
					// Session host is found and its status is 'Available'
					return sessionHost, nil
				}
			}
		}

		// Check if we've exceeded the timeout
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for session host to become available")
		}

		// Wait for the next polling interval
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			// Continue polling
		}
	}
}

// CanAssignUserToHostPool checks if the specified user is already assigned to a session host in the given host pool.
func (avd *AzureVirtualDesktopManager) CanAssignUserToHostPool(ctx context.Context, rgName, hostPoolName, userName string) (bool, error) {
	pager := avd.sessionHostsClient.NewListPager(rgName, hostPoolName, nil)

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to list session hosts: %w", err)
		}

		for _, sessionHost := range page.Value {
			if sessionHost.Properties != nil && sessionHost.Properties.AssignedUser != nil {
				if strings.EqualFold(*sessionHost.Properties.AssignedUser, userName) {
					return false, nil // User is already assigned to this session host
				}
			}
		}
	}

	return true, nil // User is not assigned to any session host in the host pool
}

func GenerateNextName(suffixes []string, maxSequences int) (string, error) {
	if len(suffixes) == 0 {
		newName := phoneticAlphabet[0]
		return newName, nil
	}

	var highestSuffix string
	for _, suffix := range suffixes {
		if suffix != "" {
			suffix = strings.ToUpper(suffix)
			if suffix > highestSuffix {
				highestSuffix = suffix
			}
		}
	}

	nextSuffix, err := getNextPhoneticWord(highestSuffix, maxSequences)
	if err != nil {
		return "", err
	}

	return nextSuffix, nil
}

var phoneticAlphabet = []string{
	"ALPHA", "BRAVO", "CHARLIE", "DELTA", "ECHO", "FOXTROT", "GOLF", "HOTEL",
	"INDIA", "JULIET", "KILO", "LIMA", "MIKE", "NOVEMBER", "OSCAR", "PAPA", "QUEBEC",
	"ROMEO", "SIERRA", "TANGO", "UNIFORM", "VICTOR", "WHISKEY", "XRAY", "YANKEE", "ZULU",
}

// generateNextWord generates the next word in the phonetic sequence given the current word and max sequences.
func getNextPhoneticWord(current string, maxSequences int) (string, error) {
	parts := strings.Split(current, "-")
	if len(parts) > maxSequences {
		return "", fmt.Errorf("Current word exceeds max sequences param")
	}

	lastWord := parts[len(parts)-1]
	index := indexOf(lastWord, phoneticAlphabet)
	if index == -1 {
		return "", fmt.Errorf("Invalid current word")
	}

	if index < len(phoneticAlphabet)-1 {
		parts[len(parts)-1] = phoneticAlphabet[index+1]
	} else {
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] != phoneticAlphabet[len(phoneticAlphabet)-1] {
				parts[i] = phoneticAlphabet[indexOf(parts[i], phoneticAlphabet)+1]
				break
			} else {
				parts[i] = phoneticAlphabet[0]
				if i == 0 {
					if len(parts) < maxSequences {
						parts = append([]string{phoneticAlphabet[0]}, parts...)
					} else {
						return "", fmt.Errorf("Max sequences exceeded")
					}
				}
			}
		}
	}

	output := strings.Join(parts, "-")
	return output, nil
}

// indexOf returns the index of a word in the phonetic alphabet.
func indexOf(word string, list []string) int {
	for i, w := range list {
		if w == word {
			return i
		}
	}
	return -1
}

// CreateHostPool creates a new host pool.
func (avd *AzureVirtualDesktopManager) CreateHostPool(ctx context.Context, rgName, suffix string) (*armdesktopvirtualization.HostPool, error) {
	hostPoolName := hostPoolNamePrefix + suffix

	// Expiration time can be 1 hour to 27 days. We'll use 25 days.
	expirationTime := time.Now().AddDate(0, 0, 25) // 25 days from now

	newHostPool := armdesktopvirtualization.HostPool{
		Location: to.Ptr(string(avd.credentials.Region)),
		Properties: &armdesktopvirtualization.HostPoolProperties{
			FriendlyName: to.Ptr("Host Pool " + suffix),
			Description:  to.Ptr("Generated via cloudy-azure"),
			HostPoolType: to.Ptr(armdesktopvirtualization.HostPoolTypePersonal),
			RegistrationInfo: &armdesktopvirtualization.RegistrationInfo{
				ExpirationTime:             &expirationTime,
				RegistrationTokenOperation: to.Ptr(armdesktopvirtualization.RegistrationTokenOperationUpdate),
			},
		},
	}

	resp, err := avd.hostPoolsClient.CreateOrUpdate(ctx, rgName, hostPoolName, newHostPool, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create host pool: %w", err)
	}

	return &resp.HostPool, nil
}

// CreateApplicationGroup creates an application group for the given host pool.
func (avd *AzureVirtualDesktopManager) CreateApplicationGroup(ctx context.Context, rgName, suffix string) (*armdesktopvirtualization.ApplicationGroup, error) {
	appGroupName := appGroupNamePrefix + suffix
	hostPoolName := hostPoolNamePrefix + suffix

	hostPoolArmPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DesktopVirtualization/hostPools/%s",
		avd.credentials.SubscriptionID, rgName, hostPoolName)

	appGroup := armdesktopvirtualization.ApplicationGroup{
		Location: to.Ptr(string(avd.credentials.Region)),
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

// CreateWorkspace creates a new workspace for the given host pool.
func (avd *AzureVirtualDesktopManager) CreateWorkspace(ctx context.Context, rgName, suffix, appGroupName string) (*armdesktopvirtualization.Workspace, error) {
	workspaceName := workspaceNamePrefix + suffix

	appGroupPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DesktopVirtualization/applicationgroups/%s",
		avd.credentials.SubscriptionID, rgName, appGroupName)

	appGroups := []*string{
		&appGroupPath,
	}

	newWorkspace := armdesktopvirtualization.Workspace{
		Location: to.Ptr(string(avd.credentials.Region)),
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
