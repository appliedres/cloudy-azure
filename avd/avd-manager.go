package avd

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"

	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	msgraphauth "github.com/microsoft/kiota-authentication-azure-go"

	"github.com/pkg/errors"

	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"

	cloudyazure "github.com/appliedres/cloudy-azure"
)

type AzureVirtualDesktopManager struct {
	Name string

	Credentials *cloudyazure.AzureCredentials
	Config      *AzureVirtualDesktopManagerConfig

	workspacesClient        *armdesktopvirtualization.WorkspacesClient
	hostPoolsClient         *armdesktopvirtualization.HostPoolsClient
	sessionHostsClient      *armdesktopvirtualization.SessionHostsClient
	userSessionsClient      *armdesktopvirtualization.UserSessionsClient
	applicationGroupsClient *armdesktopvirtualization.ApplicationGroupsClient
	applicationsClient      *armdesktopvirtualization.ApplicationsClient
	desktopsClient          *armdesktopvirtualization.DesktopsClient

	roleAssignmentsClient   *armauthorization.RoleAssignmentsClient
	graphClient		  	    *msgraphsdk.GraphServiceClient
	

	stackMutex sync.Mutex // blocks concurrent host pool creation/deletion
	lockMap    sync.Map   // used to block a user from having concurrent registrations in a single host pool
}

func NewAzureVirtualDesktopManager(ctx context.Context, name string, credentials *cloudyazure.AzureCredentials, config *AzureVirtualDesktopManagerConfig) (*AzureVirtualDesktopManager, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "NewAzureVirtualDesktopManager started")
	defer log.DebugContext(ctx, "NewAzureVirtualDesktopManager complete")

	avd := &AzureVirtualDesktopManager{
		Name:        name,
		Credentials: credentials,
		Config:      config,
	}
	err := avd.Configure(ctx)
	if err != nil {
		return nil, err
	}

	err = avd.Initialize(ctx)
	if err != nil {
		return nil, err
	}

	return avd, nil
}

// until the SDK is updated, this injects the header that ARM now requires for list calls.
// TODO: remove custom header when SDK fixes this issue
type addTargetLocations struct {
    value string
}
func (p addTargetLocations) Do(req *policy.Request) (*http.Response, error) {
    req.Raw().Header.Set("x-ms-arm-resource-list-target-locations", p.value)
    return req.Next()
}

// Handles the configuration of the Azure Virtual Desktop Manager
func (avd *AzureVirtualDesktopManager) Configure(ctx context.Context) error {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "AVD Manager - Configure started")
	defer log.DebugContext(ctx, "AVD Manager - Configure complete")

	cred, err := cloudyazure.NewAzureCredentials(avd.Credentials)
	if err != nil {
		return err
	}

	baseOptions := arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
			PerCallPolicies: []policy.Policy{
				addTargetLocations{value: avd.Credentials.Region},  // TODO: remove when SDK fixes this
			},
		},
	}

	clientFactory, err := armdesktopvirtualization.NewClientFactory(avd.Credentials.SubscriptionID, cred, &baseOptions)
	if err != nil {
		return err
	}

	avd.workspacesClient = clientFactory.NewWorkspacesClient()
	avd.hostPoolsClient = clientFactory.NewHostPoolsClient()
	avd.sessionHostsClient = clientFactory.NewSessionHostsClient()
	avd.userSessionsClient = clientFactory.NewUserSessionsClient()
	avd.applicationGroupsClient = clientFactory.NewApplicationGroupsClient()
	avd.applicationsClient = clientFactory.NewApplicationsClient()
	avd.desktopsClient = clientFactory.NewDesktopsClient()

	roleassignmentsclient, err := armauthorization.NewRoleAssignmentsClient(avd.Credentials.SubscriptionID, cred, &baseOptions)
	if err != nil {
		return err
	}
	avd.roleAssignmentsClient = roleassignmentsclient

	// Setup MS Graph client
	credGraph, err := azidentity.NewClientSecretCredential(avd.Credentials.TenantID, avd.Credentials.ClientID, avd.Credentials.ClientSecret, nil)
	if err != nil {
		return err
	}

	authProv, err := msgraphauth.NewAzureIdentityAuthenticationProviderWithScopes(
		credGraph,
		[]string{"https://graph.microsoft.us/.default"}, // Gov Graph
	)
	if err != nil {
		return err
	}
	
	adapter, err := msgraphsdk.NewGraphRequestAdapter(authProv)
	if err != nil {
		return err
	}
	adapter.SetBaseUrl("https://graph.microsoft.us/v1.0")
	avd.graphClient = msgraphsdk.NewGraphServiceClient(adapter)

	// AVD resource Naming conventions
	avd.Config.PersonalHostPoolNamePrefix = avd.Config.PrefixBase + "-HP-Personal-"
	avd.Config.PersonalWorkspaceNamePrefix = avd.Config.PrefixBase + "-WS-Personal-"
	avd.Config.PersonalAppGroupNamePrefix = avd.Config.PrefixBase + "-AG-Personal-"

	avd.Config.PooledHostPoolNamePrefix = avd.Config.PrefixBase + "-HP-Pooled-"
	avd.Config.PooledWorkspaceNamePrefix = avd.Config.PrefixBase + "-WS-Pooled-"
	avd.Config.PooledAppGroupNamePrefix = avd.Config.PrefixBase + "-AG-Pooled-"

	// TODO: ensure all AVD resources with this PrefixBase fit into these naming conventions, cleanup those that do not

	return nil
}

func (avd *AzureVirtualDesktopManager) Initialize(ctx context.Context) error {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "AVD Manager - Initialize started")
	defer log.DebugContext(ctx, "AVD Manager - Initialize complete")

	// TODO: make AVD pool settings configurable
	// TODO: if config . linux VMs enabled

	// TODO: fix conflict of pooled vs personal host pools
	_, _, err := avd.EnsurePooledStack(ctx, armdesktopvirtualization.LoadBalancerTypeBreadthFirst, 3)
	if err != nil {
		return logging.LogAndWrapErr(ctx, log, err, "Failed to ensure AVD pooled stack")
	}

	return nil
}

// Lock key format: vm.UserID + hostPoolName
func (avd *AzureVirtualDesktopManager) acquireHostPoolLockForUser(ctx context.Context, vmUserID, hostPoolName string) bool {
	log := logging.GetLogger(ctx)

	lockKey := fmt.Sprintf("%s-%s", vmUserID, hostPoolName)
	// Attempt to acquire the lock by checking if the key exists in the map
	if _, exists := avd.lockMap.Load(lockKey); exists {
		log.DebugContext(ctx, "Unable to acquire host pool lock for user", "lockKey", lockKey, "UserID", vmUserID, "HostPoolName", hostPoolName)
		return false // The lock already exists, so the registration is in progress.
	}
	// If the lock doesn't exist, create it to block other requests.
	avd.lockMap.Store(lockKey, struct{}{})
	log.DebugContext(ctx, "Successfully acquired host pool lock for user", "UserID", vmUserID, "HostPoolName", hostPoolName)
	return true
}

// Release the lock once the registration process is done.
func (avd *AzureVirtualDesktopManager) releaseHostPoolLockForUser(ctx context.Context, vmUserID, hostPoolName string) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "Releasing host pool lock for user", "UserID", vmUserID, "HostPoolName", hostPoolName)

	lockKey := fmt.Sprintf("%s-%s", vmUserID, hostPoolName)
	avd.lockMap.Delete(lockKey)
}

// Prior to VM registration, this process generates a token for a given host pool.
// This token will later be used in the registration process to join the VM to the host pool.
// The user is also assigned to the related desktop application group.
func (avd *AzureVirtualDesktopManager) PreRegister(ctx context.Context, vm *models.VirtualMachine) (hostPoolName, token *string, err error) {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "Starting AVD PreRegister", "VM", vm.ID)

	osType := vm.Template.OperatingSystem
	if osType != models.VirtualMachineTemplateOperatingSystemWindows {
		log.WarnContext(ctx, "Unsupported OS type for AVD registration", "OS", osType)
		return nil, nil, fmt.Errorf("unsupported OS type; only Windows is supported for AVD registration")
	}

	avd.stackMutex.Lock()
	defer avd.stackMutex.Unlock()
	log.DebugContext(ctx, "AVD PreRegister - cleared AVD stack lock")

	// Step 1: Check existing host pools
	hpFilter := avd.Config.PersonalHostPoolNamePrefix
	log.DebugContext(ctx, "Retrieving host pools", "Filter", hpFilter)

	hostPools, err := avd.listHostPools(ctx, &hpFilter)
	if err != nil {
		log.ErrorContext(ctx, "Failed to retrieve host pools", "Error", err)
		return nil, nil, fmt.Errorf("failed to retrieve host pools: %w", err)
	}
	log.DebugContext(ctx, "Retrieved host pools", "Count", len(hostPools))

	avd.sortHostPoolsByPhoneticSuffix(hostPools)
	log.DebugContext(ctx, "Sorted host pools by phonetic suffix", "Count", len(hostPools))

	// Check if the user can be assigned to any existing host pool
	var targetHostPool *armdesktopvirtualization.HostPool
	for _, pool := range hostPools {
		log := log.With("HostPool", *pool.Name, "UserID", vm.UserID) // shrink repetitive fields

		// determine AVD stack suffix
		suffix := strings.TrimPrefix(*pool.Name, avd.Config.PersonalHostPoolNamePrefix)
		if suffix == "" {
			log.WarnContext(ctx, "host pool name is empty after trimming prefix, skipping…")
			continue
		}

		// valid desktop app group
		appGroupName := avd.Config.PersonalAppGroupNamePrefix + suffix
		desktopAppGroup, err := avd.GetDesktopAppGroupByName(ctx, appGroupName)
		if err != nil {
			log.WarnContext(ctx, "no valid desktop app group, skipping…", "AppGroupName", appGroupName, "Error", err)
			continue
		}
		if desktopAppGroup == nil {
			log.WarnContext(ctx, "desktop app group is nil, skipping…")
			continue
		}
		log.DebugContext(ctx, "desktop app group OK", "AppGroupName", *desktopAppGroup.Name)

		// valid workspace
		workspaceName := avd.Config.PersonalWorkspaceNamePrefix + suffix
		workspace, err := avd.GetWorkspaceByName(ctx, workspaceName)
		if err != nil {
			log.WarnContext(ctx, "no valid workspace, skipping…", "WorkspaceName", workspaceName, "Error", err)
			continue
		}
		log.DebugContext(ctx, "workspace OK", "WorkspaceName", *workspace.Name)

		// user can be assigned
		canAssign, err := avd.CanAssignUserToHostPool(ctx, *pool.Name, vm.UserID)
		if err != nil {
			log.WarnContext(ctx, "error checking assignment, skipping…", "Error", err)
			continue
		}
		if !canAssign {
			log.DebugContext(ctx, "user cannot be assigned, skipping…")
			continue
		}
		log.DebugContext(ctx, "user can be assigned")

		// acquire lock on host pool for this user
		if !avd.acquireHostPoolLockForUser(ctx, vm.UserID, *pool.Name) {
			log.DebugContext(ctx, "unable to acquire lock, skipping…")
			continue
		}

		// successfully found existing host pool
		log.DebugContext(ctx, "successfully found existing host pool for session host assignment")
		targetHostPool = pool
		break
	}

	// Step 2: If no suitable host pool exists, create a new one
	if targetHostPool == nil {
		log.InfoContext(ctx, "No suitable host pool found; creating new host pool")

		var nameSuffix string
		if len(hostPools) == 0 {
			nameSuffix = "ALPHA"
		} else {
			highestHostPoolName := hostPools[len(hostPools)-1].Name
			highestSuffix := strings.TrimPrefix(*highestHostPoolName, avd.Config.PersonalHostPoolNamePrefix)
			var err error
			nameSuffix, err = GenerateNextName(highestSuffix, 2)
			if err != nil {
				log.ErrorContext(ctx, "Failed to generate new host pool name", "Error", err)
				return nil, nil, fmt.Errorf("failed to generate new host pool name: %w", err)
			}
		}

		log.InfoContext(ctx, "Creating new AVD stack", "Suffix", nameSuffix)

		targetHostPool, _, _, err = avd.createAvdStack(ctx, nameSuffix)
		if err != nil {
			log.ErrorContext(ctx, "Failed to create AVD resource stack", "Suffix", nameSuffix, "Error", err)
			return nil, nil, fmt.Errorf("failed to create AVD resource stack: %w", err)
		}

		if !avd.acquireHostPoolLockForUser(ctx, vm.UserID, *targetHostPool.Name) {
			log.ErrorContext(ctx, "Failed to acquire lock on newly created host pool", "HostPool", *targetHostPool.Name, "UserID", vm.UserID)
			return nil, nil, fmt.Errorf("failed to acquire lock on newly created host pool [%s] for user [%s]: %w",
				*targetHostPool.Name, vm.UserID, err)
		}

		log.InfoContext(ctx, "Successfully created new AVD stack", "suffix", nameSuffix, "UserID", vm.UserID)
	}

	// Step 3: Retrieve registration token
	log.DebugContext(ctx, "Retrieving registration token", "HostPool", *targetHostPool.Name)
	token, err = avd.RetrieveRegistrationToken(ctx, *targetHostPool.Name)
	if err != nil {
		log.ErrorContext(ctx, "Failed to retrieve registration token", "HostPool", *targetHostPool.Name, "Error", err)
		avd.releaseHostPoolLockForUser(ctx, vm.UserID, *targetHostPool.Name)
		return nil, nil, fmt.Errorf("failed to retrieve registration token: %w", err)
	}

	log.InfoContext(ctx, "AVD PreRegister complete, successfully retrieved registration token", "HostPool", *targetHostPool.Name)

	return targetHostPool.Name, token, nil
}

// After registering, the user must then be assigned to the new session host
func (avd *AzureVirtualDesktopManager) PostRegister(ctx context.Context, vm *models.VirtualMachine, hpName string) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)

	log.InfoContext(ctx, "Starting PostRegister process", "VM", vm.ID, "HostPoolName", hpName)

	// Release the lock acquired in pre-register
	defer func() {
		avd.releaseHostPoolLockForUser(ctx, vm.UserID, hpName)
	}()

	log.DebugContext(ctx, "Waiting for session host to be ready", "HostPoolName", hpName, "VM", vm.ID)
	sessionHost, err := avd.WaitForSessionHost(ctx, hpName, vm.ID, 10*time.Minute)
	if err != nil {
		log.WarnContext(ctx, "Error waiting for session host", "Error", err)
		return nil, logging.LogAndWrapErr(ctx, log, err, "Waiting for session host to be ready")
	}

	log.DebugContext(ctx, "Session host ready", "SessionHostName", *sessionHost.Name)

	_, sessionHostName, _, err := avd.ParseSessionHostName(ctx, sessionHost)
	if err != nil {
		log.WarnContext(ctx, "Error parsing session host name", "Error", err)
		return nil, err
	}

	// Assign session host to user
	log.InfoContext(ctx, "Assigning session host to user", "SessionHostName", sessionHostName, "UserID", vm.UserID)
	err = avd.AssignSessionHost(ctx, hpName, sessionHostName, vm.UserID)
	if err != nil {
		log.WarnContext(ctx, "Error assigning session host to user", "Error", err)
		return nil, err
	}

	// Find desktop application group from host pool
	log.DebugContext(ctx, "Retrieving desktop application group", "HostPoolName", hpName)
	desktopApplicationGroup, err := avd.GetDesktopApplicationGroupFromHostpool(ctx, hpName)
	if err != nil {
		log.WarnContext(ctx, "Error retrieving desktop application group", "Error", err)
		return nil, err
	}

	// Determine workspace from desktop application group
	log.DebugContext(ctx, "Determining workspace from desktop application group")
	workspacePathSegments := strings.Split(*desktopApplicationGroup.Properties.WorkspaceArmPath, "/")
	workspaceName := workspacePathSegments[len(workspacePathSegments)-1]
	log.DebugContext(ctx, "Parsed workspace name", "WorkspaceName", workspaceName)

	workspace, err := avd.GetWorkspaceByName(ctx, workspaceName)
	if err != nil {
		log.WarnContext(ctx, "Error retrieving workspace", "Error", err)
		return nil, err
	}
	workspaceID := *workspace.Properties.ObjectID
	log.DebugContext(ctx, "Retrieved workspace ID", "WorkspaceID", workspaceID)

	// Find the resource ID of the desktop application
	log.DebugContext(ctx, "Retrieving desktop application", "DesktopApplicationGroup", *desktopApplicationGroup.Name)
	desktop, err := avd.getSingleDesktop(ctx, *desktopApplicationGroup.Name)
	if err != nil {
		log.WarnContext(ctx, "Error retrieving desktop application", "Error", err)
		return nil, err
	}
	resourceID := *desktop.Properties.ObjectID
	log.DebugContext(ctx, "Retrieved desktop application resource ID", "ResourceID", resourceID)

	// Generate connection URL
	log.DebugContext(ctx, "Generating Windows client URI", "avdUriVersion", avd.Config.UriVersion, "UseMultiMon", avd.Config.UseMultipleMonitors)
	connection := &models.VirtualMachineConnection{
		RemoteDesktopProvider: "AVD",
		URL:                   avd.GenerateWindowsClientURI(workspaceID, resourceID, vm.UserID, avd.Config.UriEnv, avd.Config.UriVersion, toBool(avd.Config.UseMultipleMonitors)),
	}

	vm.Connect = connection
	log.InfoContext(ctx, "PostRegister process completed successfully", "VM", vm.ID)

	return vm, nil
}

// To be completed when a VM is deleted.
// Removes the session host and cleans up empty host pools / app groups / workspaces
func (avd *AzureVirtualDesktopManager) Cleanup(ctx context.Context, vmID string) error {
	log := logging.GetLogger(ctx)

	hpFilter := avd.Config.PersonalHostPoolNamePrefix
	log.InfoContext(ctx, "Starting AVD Cleanup process", "vmID", vmID, "hostPoolFilter", hpFilter)

	// acquire lock so we don't have concurrency issues with other threads deleting or creating host pools
	avd.stackMutex.Lock()
	defer avd.stackMutex.Unlock()
	log.DebugContext(ctx, "AVD cleanup - cleared AVD stack lock")

	// Step 1: Check existing host pools
	hostPools, err := avd.listHostPools(ctx, &hpFilter)
	if err != nil {
		return fmt.Errorf("AVD Cleanup - failed to retrieve host pools: %w", err)
	}
	log.DebugContext(ctx, "Retrieved host pools", "hostPoolCount", len(hostPools))

	// Find and delete the session host associated with the VM
	for _, hostPool := range hostPools {
		if hostPool.Name == nil {
			log.WarnContext(ctx, "Encountered host pool with nil name; skipping")
			continue
		}

		log.DebugContext(ctx, "Searching for a VM's session host in this host pool in order to delete", "VMID", vmID, "hostPoolName", *hostPool.Name)
		sessionHostsPager := avd.sessionHostsClient.NewListPager(avd.Credentials.ResourceGroup, *hostPool.Name, nil)
		for sessionHostsPager.More() {
			page, err := sessionHostsPager.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("error listing session hosts in host pool %s: %w", *hostPool.Name, err)
			}

			for _, sessionHost := range page.Value {
				if sessionHost.Name != nil && strings.Contains(*sessionHost.Name, vmID) {
					// Extract sessionHostName from "hostpoolName/sessionHostName"
					sessionHostParts := strings.Split(*sessionHost.Name, "/")
					if len(sessionHostParts) != 2 {
						return fmt.Errorf("found session host with VM ID in name, but its name format is invalid. sessionHost.Name=%s. err=%v", *sessionHost.Name, err)
					}
					sessionHostName := sessionHostParts[1]

					log.InfoContext(ctx, "Found session host associated with VM; deleting", "sessionHost", sessionHostName, "hostPoolName", *hostPool.Name)
					_, err := avd.sessionHostsClient.Delete(ctx, avd.Credentials.ResourceGroup, *hostPool.Name, sessionHostName, nil)
					if err != nil {
						return fmt.Errorf("error deleting session host %s in host pool %s: %w", sessionHostName, *hostPool.Name, err)
					}
					log.InfoContext(ctx, "Deleted session host", "sessionHost", sessionHostName, "hostPoolName", *hostPool.Name)
				}
			}
		}
	}

	// Step 2: Sort host pools by phonetic suffix
	log.InfoContext(ctx, "Sorting host pools by name")
	sort.Slice(hostPools, func(i, j int) bool {
		return *hostPools[i].Name < *hostPools[j].Name
	})

	// Step 3: Identify empty host pools after the last non-empty host pool
	var deleteHostPoolNames []string
	for i := len(hostPools) - 1; i >= 0; i-- {
		hostPool := hostPools[i]
		if hostPool.Name == nil {
			log.WarnContext(ctx, "Found host pool with nil name; skipping")
			continue
		}

		log.DebugContext(ctx, "Checking if host pool is empty", "hostPoolName", *hostPool.Name)
		isEmpty, err := avd.isHostPoolEmpty(ctx, *hostPool.Name)
		if err != nil {
			return fmt.Errorf("error checking if host pool %s is empty: %w", *hostPool.Name, err)
		}

		if !isEmpty {
			log.InfoContext(ctx, "Found non-empty host pool; stopping deletion collection", "hostPoolName", *hostPool.Name)
			break
		}

		log.InfoContext(ctx, "Identified empty host pool for deletion", "hostPoolName", *hostPool.Name)
		deleteHostPoolNames = append(deleteHostPoolNames, *hostPool.Name)
	}

	// If there are empty host pools to delete, pop the last one (leaving one empty)
	// This host pool will remain available for new connections, so we shouldn't have to worry
	// about users joining a host pool that is being deleted.
	// Given "-DELTA", "-CHARLIE" and "-BRAVO" empty host pools, this will not delete "-BRAVO"
	if len(deleteHostPoolNames) > 1 {
		deleteHostPoolNames = deleteHostPoolNames[:len(deleteHostPoolNames)-1]
	}

	// Step 4: Delete the collected host pools
	log.InfoContext(ctx, "Deleting empty host pools", "hostPoolsToDelete", deleteHostPoolNames)
	for _, hostPoolNameToDelete := range deleteHostPoolNames {
		log.InfoContext(ctx, "Deleting host pool and its associated resources", "hostPoolName", hostPoolNameToDelete)
		err := avd.deleteStack(ctx, hostPoolNameToDelete)
		if err != nil {
			return fmt.Errorf("error deleting host pool %s and its associated resources: %w", hostPoolNameToDelete, err)
		}
		log.InfoContext(ctx, "Successfully deleted host pool and its resources", "hostPoolName", hostPoolNameToDelete)
	}

	log.InfoContext(ctx, "AVD Cleanup process completed successfully", "vmID", vmID)
	return nil
}

// createAvdStack creates a new AVD stack including the host pool, application group, and workspace.
func (avd *AzureVirtualDesktopManager) createAvdStack(ctx context.Context, suffix string) (
	*armdesktopvirtualization.HostPool, *armdesktopvirtualization.ApplicationGroup, *armdesktopvirtualization.Workspace, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "Creating AVD stack", "Suffix", suffix)
	defer log.DebugContext(ctx, "AVD stack creation complete")

	resourceGroupName := avd.Credentials.ResourceGroup

	tags := map[string]*string{
		"stack_group_suffix": to.Ptr(suffix),
		"arkloud_created_by": to.Ptr("cloudy-azure"),
	}

	log.DebugContext(ctx, "Creating host pool", "ResourceGroup", resourceGroupName, "Suffix", suffix)
	// Create host pool
	hostPool, err := avd.CreateHostPool(ctx, suffix, tags)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create host pool", "Error", err)
		return nil, nil, nil, fmt.Errorf("failed to create host pool: %w", err)
	}
	log.DebugContext(ctx, "Host pool created successfully", "HostPoolName", *hostPool.Name)

	log.DebugContext(ctx, "Creating application group linked to the host pool", "ResourceGroup", resourceGroupName, "Suffix", suffix)
	// Create application group linked to the new host pool
	desktopAppGroup, err := avd.CreatePersonalDesktopApplicationGroup(ctx, suffix, tags)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create application group", "Error", err)
		// TODO: delete resources during failed stack creation
		return nil, nil, nil, fmt.Errorf("failed to create application group: %w", err)
	}
	log.DebugContext(ctx, "Application group created successfully", "AppGroupName", *desktopAppGroup.Name)

	log.DebugContext(ctx, "Renaming desktop application", "ResourceGroup", resourceGroupName, "AppGroupName", *desktopAppGroup.Name, "Suffix", suffix, "DesktopNamePrefix", avd.Config.DesktopNamePrefix)
	desktopApp, err := avd.renameDesktop(ctx, *desktopAppGroup.Name, suffix, avd.Config.DesktopNamePrefix)
	if err != nil {
		log.ErrorContext(ctx, "Error renaming desktop application", "Error", err)
		return nil, nil, nil, errors.Wrap(err, "error renaming desktopApp during stack creation")
	}
	log.DebugContext(ctx, "Desktop application renamed successfully", "DesktopApp", desktopApp)

	log.DebugContext(ctx, "Creating workspace linked to the desktop application group", "ResourceGroup", resourceGroupName, "Suffix", suffix, "AppGroupName", *desktopAppGroup.Name)
	// Create workspace linked to the desktop application group
	workspace, err := avd.CreatePersonalWorkspaceForAppGroup(ctx, suffix, *desktopAppGroup.Name, tags)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create workspace", "Error", err)
		// TODO: delete resources during failed stack creation
		return nil, nil, nil, fmt.Errorf("failed to create workspace: %w", err)
	}
	log.DebugContext(ctx, "Workspace created successfully", "WorkspaceName", *workspace.Name)

	log.DebugContext(ctx, "Assigning AVD user group to the desktop application group", "AppGroupName", *desktopAppGroup.Name)
	// Assign AVD user group to the new desktop application group (DAG)
	err = avd.AssignAVDUserGroupToAppGroup(ctx, *desktopAppGroup.Name)
	if err != nil {
		log.ErrorContext(ctx, "Failed to assign AVD user group to desktop application group", "AppGroupName", *desktopAppGroup.Name, "Error", err)
		// TODO: delete resources during failed stack creation
		return nil, nil, nil, fmt.Errorf("unable to assign AVD User group to Desktop Application Group [%s]", *desktopAppGroup.Name)
	}
	log.DebugContext(ctx, "AVD user group assigned successfully", "AppGroupName", *desktopAppGroup.Name)

	log.InfoContext(ctx, "AVD stack created successfully", "HostPoolName", *hostPool.Name, "AppGroupName", *desktopAppGroup.Name, "WorkspaceName", *workspace.Name)
	return hostPool, desktopAppGroup, workspace, nil
}

func toBool(input string) bool {
	output, err := strconv.ParseBool(input)
	if err != nil {
		return false
	}
	return output
}
