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
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"

	msgraphauth "github.com/microsoft/kiota-authentication-azure-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"

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

	roleAssignmentsClient *armauthorization.RoleAssignmentsClient
	graphClient           *msgraphsdk.GraphServiceClient

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
				addTargetLocations{value: avd.Credentials.Region}, // TODO: remove when SDK fixes this
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
func (avd *AzureVirtualDesktopManager) AcquireHostPoolLockForUser(ctx context.Context, vmUserID, hostPoolName string) bool {
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
func (avd *AzureVirtualDesktopManager) ReleaseHostPoolLockForUser(ctx context.Context, vmUserID, hostPoolName string) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "Releasing host pool lock for user", "UserID", vmUserID, "HostPoolName", hostPoolName)

	lockKey := fmt.Sprintf("%s-%s", vmUserID, hostPoolName)
	avd.lockMap.Delete(lockKey)
}

// After registering, the user must then be assigned to the new session host
func (avd *AzureVirtualDesktopManager) PostRegister(ctx context.Context, vm *models.VirtualMachine, hpName string) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)

	log.InfoContext(ctx, "Starting PostRegister process", "VM", vm.ID, "HostPoolName", hpName)

	// Release the lock acquired in pre-register
	defer func() {
		avd.ReleaseHostPoolLockForUser(ctx, vm.UserID, hpName)
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
	hostPools, err := avd.ListHostPools(ctx, &hpFilter)
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

func toBool(input string) bool {
	output, err := strconv.ParseBool(input)
	if err != nil {
		return false
	}
	return output
}
