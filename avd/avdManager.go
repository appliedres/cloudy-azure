package avd

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	"github.com/pkg/errors"
)

type AzureVirtualDesktopManager struct {
	credentials *cloudyazure.AzureCredentials
	config      *cloudyazure.AzureVirtualDesktopConfig

	workspacesClient   *armdesktopvirtualization.WorkspacesClient
	hostPoolsClient    *armdesktopvirtualization.HostPoolsClient
	sessionHostsClient *armdesktopvirtualization.SessionHostsClient
	userSessionsClient *armdesktopvirtualization.UserSessionsClient
	appGroupsClient    *armdesktopvirtualization.ApplicationGroupsClient
	appsClient         *armdesktopvirtualization.ApplicationsClient
	desktopsClient     *armdesktopvirtualization.DesktopsClient

	roleAssignmentsClient *armauthorization.RoleAssignmentsClient

	stackMutex sync.Mutex // blocks concurrent host pool creation/deletion
	lockMap    sync.Map   // used to block a user from having concurrent registrations in a single host pool
}

func NewAzureVirtualDesktopManager(ctx context.Context, credentials *cloudyazure.AzureCredentials, config *cloudyazure.AzureVirtualDesktopConfig) (*AzureVirtualDesktopManager, error) {
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
	cred, err := cloudyazure.NewAzureCredentials(avd.credentials)
	if err != nil {
		return err
	}

	baseOptions := arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	}

	avdOptions := baseOptions
	avdOptions.APIVersion = "2023-09-05" // Important! Latest AVD API version is not supported in Azure Govt
	clientFactory, err := armdesktopvirtualization.NewClientFactory(avd.credentials.SubscriptionID, cred, &avdOptions)
	if err != nil {
		return err
	}

	avd.workspacesClient = clientFactory.NewWorkspacesClient()
	avd.hostPoolsClient = clientFactory.NewHostPoolsClient()
	avd.sessionHostsClient = clientFactory.NewSessionHostsClient()
	avd.userSessionsClient = clientFactory.NewUserSessionsClient()
	avd.appGroupsClient = clientFactory.NewApplicationGroupsClient()
	avd.appsClient = clientFactory.NewApplicationsClient()
	avd.desktopsClient = clientFactory.NewDesktopsClient()

	roleassignmentsclient, err := armauthorization.NewRoleAssignmentsClient(avd.credentials.SubscriptionID, cred, &baseOptions)
	if err != nil {
		return err
	}
	avd.roleAssignmentsClient = roleassignmentsclient

	avd.config.HostPoolNamePrefix = avd.config.PrefixBase + "-HP-"
	avd.config.WorkspaceNamePrefix = avd.config.PrefixBase + "-WS-"
	avd.config.AppGroupNamePrefix = avd.config.PrefixBase + "-DAG-"

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
	if osType != "windows" {
		log.WarnContext(ctx, "Unsupported OS type for AVD registration", "OS", osType)
		return nil, nil, fmt.Errorf("unsupported OS type; only Windows is supported for AVD registration")
	}

	rgName := avd.credentials.ResourceGroup

	avd.stackMutex.Lock()
	defer avd.stackMutex.Unlock()
	log.DebugContext(ctx, "AVD PreRegister - cleared AVD stack lock")

	// Step 1: Check existing host pools
	hpFilter := avd.config.HostPoolNamePrefix
	log.DebugContext(ctx, "Retrieving host pools", "ResourceGroup", rgName, "Filter", hpFilter)

	hostPools, err := avd.listHostPools(ctx, rgName, &hpFilter)
	if err != nil {
		log.ErrorContext(ctx, "Failed to retrieve host pools", "Error", err)
		return nil, nil, fmt.Errorf("failed to retrieve host pools: %w", err)
	}

	log.DebugContext(ctx, "Retrieved host pools", "Count", len(hostPools))

	// TODO: check that host pool is valid, with necessary resources (WS, DAG). It may have previously failed creation

	// Check if the user can be assigned to any existing host pool
	var targetHostPool *armdesktopvirtualization.HostPool
	var hostPoolSuffixes []string
	for _, pool := range hostPools {
		log.DebugContext(ctx, "Checking if user can be assigned to host pool", "HostPool", *pool.Name, "UserID", vm.UserID)

		canAssign, err := avd.CanAssignUserToHostPool(ctx, rgName, *pool.Name, vm.UserID)
		if err != nil {
			log.WarnContext(ctx, "Error checking user assignment in host pool", "HostPool", *pool.Name, "Error", err)
			return nil, nil, fmt.Errorf("error checking assignments in host pool")
		}

		if canAssign {
			log.DebugContext(ctx, "Can assign user to host pool", "HostPool", *pool.Name, "UserID", vm.UserID)
			if avd.acquireHostPoolLockForUser(ctx, vm.UserID, *pool.Name) {
				log.InfoContext(ctx, "target host pool found", "HostPool", *pool.Name, "UserID", vm.UserID)
				targetHostPool = pool
				break
			}
			log.DebugContext(ctx, "Unable to acquire lock on host pool", "HostPool", *pool.Name, "UserID", vm.UserID)
		}

		hostPoolSuffixes = append(hostPoolSuffixes, strings.TrimPrefix(*pool.Name, avd.config.HostPoolNamePrefix))
	}

	// Step 2: If no suitable host pool exists, create a new one
	if targetHostPool == nil {
		log.InfoContext(ctx, "No suitable host pool found; creating new host pool")

		nameSuffix, err := GenerateNextName(hostPoolSuffixes, 2)
		if err != nil {
			log.ErrorContext(ctx, "Failed to generate new host pool name", "Error", err)
			return nil, nil, fmt.Errorf("failed to generate new host pool name: %w", err)
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
	token, err = avd.RetrieveRegistrationToken(ctx, rgName, *targetHostPool.Name)
	if err != nil {
		log.ErrorContext(ctx, "Failed to retrieve registration token", "HostPool", *targetHostPool.Name, "Error", err)
		avd.releaseHostPoolLockForUser(ctx, vm.UserID, *targetHostPool.Name)
		return nil, nil, fmt.Errorf("failed to retrieve registration token: %w", err)
	}

	log.InfoContext(ctx, "Successfully retrieved registration token", "HostPool", *targetHostPool.Name)

	return targetHostPool.Name, token, nil
}

func (avd *AzureVirtualDesktopManager) GetRegistrationScript(ctx context.Context, vm *models.VirtualMachine, registrationToken string) (*string, error) {
	log := logging.GetLogger(ctx)

    useOU := false
    ouPath := ""
    if avd.config.OUPath != nil {
        useOU = true
        ouPath = *avd.config.OUPath
    }

	// Define the PowerShell script with placeholders
	scriptTemplate := `
# Set up logging for verbose output
$logFilePath = "$pwd\install_log.txt"
Start-Transcript -Path $logFilePath -Append

$REGISTRATIONTOKEN = "%s"
$DOMAIN_NAME = "%s"
$DOMAIN_USERNAME = "%s"
$DOMAIN_PASSWORD = "%s"
$USE_OU = "%t"
$OU_PATH = "%s"

# Check if the machine is already in the domain
$computerDomain = (Get-WmiObject -Class Win32_ComputerSystem).Domain
if ($computerDomain -eq $DOMAIN_NAME) {
    Write-Host "Machine is already part of the domain: $DOMAIN_NAME"
} else {
    # Join the domain with or without OU
    try {
        Write-Host "Attempting to join the domain: $DOMAIN_NAME"
        $securePassword = ConvertTo-SecureString -String $DOMAIN_PASSWORD -AsPlainText -Force
        $credential = New-Object System.Management.Automation.PSCredential ($DOMAIN_USERNAME, $securePassword)
        
        if ($USE_OU -eq "True" -and $OU_PATH -ne "") {
            Write-Host "Joining with OU: $OU_PATH"
            Add-Computer -DomainName $DOMAIN_NAME -Credential $credential -OUPath $OU_PATH -Force -Verbose
        } else {
            Write-Host "Joining without specifying an OU"
            Add-Computer -DomainName $DOMAIN_NAME -Credential $credential -Force -Verbose
        }

        Write-Host "Successfully joined the domain."
    } catch {
        Write-Host "Error joining the domain: $_"
        Stop-Transcript
        return
    }
}

# Define URLs for the installers
$uris = @(
	"https://query.prod.cms.rt.microsoft.com/cms/api/am/binary/RWrmXv",   # RDAgent
	"https://query.prod.cms.rt.microsoft.com/cms/api/am/binary/RWrxrH"    # BootLoader Agent
)

$installers = @()

# Download installers
foreach ($uri in $uris) {
	try {
		Write-Host "Starting download: $uri"
		$download = Invoke-WebRequest -Uri $uri -UseBasicParsing -Verbose
		$fileName = ($download.Headers.'Content-Disposition').Split('=')[1].Replace('"','')
		$outputPath = "$pwd\$fileName"

		if (Test-Path $outputPath) {
			Write-Host "File $fileName already exists. Skipping download."
		} else {
			$output = [System.IO.FileStream]::new($outputPath, [System.IO.FileMode]::Create)
			$output.write($download.Content, 0, $download.RawContentLength)
			$output.close()
			Write-Host "Downloaded: $fileName"
		}

		$installers += $outputPath
	} catch {
		Write-Host "Error downloading ${uri}: $_"
		return
	}
}

# Unblock files after download
foreach ($installer in $installers) {
	if (Test-Path $installer) {
		Write-Host "Unblocking file: $installer"
		Unblock-File -Path $installer -Verbose
	} else {
		Write-Host "File $installer not found, skipping unblock."
	}
}

# Find the RDAgent installer
$rdaAgentInstaller = $installers | Where-Object { $_ -match "Microsoft.RDInfra.RDAgent.Installer-x64" }
if (-not $rdaAgentInstaller) {
	Write-Host "RDAgent installer not found."
	return
}

# Find the BootLoader installer
$rdaBootLoaderInstaller = $installers | Where-Object { $_ -match "Microsoft.RDInfra.RDAgentBootLoader.Installer-x64" }
if (-not $rdaBootLoaderInstaller) {
	Write-Host "BootLoader Agent installer not found."
	return
}

# Install RDAgent
Write-Host "Installing RDAgent with registration token."
try {
	Start-Process msiexec -ArgumentList "/i $rdaAgentInstaller REGISTRATIONTOKEN=$REGISTRATIONTOKEN /quiet /norestart" -Wait -Verbose -Verb RunAs
} catch {
	Write-Host "Error installing RDAgent: $_"
	return
}

# Install BootLoader Agent
Write-Host "Installing BootLoader Agent."
try {
	Start-Process msiexec -ArgumentList "/i $rdaBootLoaderInstaller /quiet" -Wait -Verbose -Verb RunAs
} catch {
	Write-Host "Error installing BootLoader Agent: $_"
	return
}

# Finalize and restart
Write-Host "Preparing for system restart."
Restart-Computer -Force

# Stop the transcript and finalize the log
Stop-Transcript
	`

	// Inject the registration key and domain credentials into the script
	script := fmt.Sprintf(scriptTemplate, registrationToken, avd.config.DomainName, avd.config.DomainUser, avd.config.DomainPass, useOU, ouPath)

	log.InfoContext(ctx, "Generated AVD registration script", "VMID", vm.ID)
	return &script, nil
}

// After registering, the user must then be assigned to the new session host
func (avd *AzureVirtualDesktopManager) PostRegister(ctx context.Context, vm *models.VirtualMachine, hpName string) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)

	log.InfoContext(ctx, "Starting PostRegister process", "VM", vm.ID, "HostPoolName", hpName)

	// Release the lock acquired in pre-register
	defer func() {
		avd.releaseHostPoolLockForUser(ctx, vm.UserID, hpName)
	}()

	rgName := avd.credentials.ResourceGroup

	log.DebugContext(ctx, "Waiting for session host to be ready", "ResourceGroup", rgName, "HostPoolName", hpName, "VM", vm.ID)
	sessionHost, err := avd.WaitForSessionHost(ctx, rgName, hpName, vm.ID, 10*time.Minute)
	if err != nil {
		log.WarnContext(ctx, "Error waiting for session host", "Error", err)
		return nil, errors.Wrap(err, "Waiting for session host to be ready")
	}

	log.DebugContext(ctx, "Session host ready", "SessionHostName", *sessionHost.Name)

	// Parse session host name
	parts := strings.SplitN(*sessionHost.Name, "/", 2)
	if len(parts) != 2 {
		err := fmt.Errorf("Could not split sessionHost.Name: %s", *sessionHost.Name)
		log.WarnContext(ctx, "Error splitting session host name", "Error", err)
		return nil, err
	}
	sessionHostName := parts[1]
	log.DebugContext(ctx, "Parsed session host name", "SessionHostName", sessionHostName)

	// Assign session host to user
	log.InfoContext(ctx, "Assigning session host to user", "SessionHostName", sessionHostName, "UserID", vm.UserID)
	err = avd.AssignSessionHost(ctx, rgName, hpName, sessionHostName, vm.UserID)
	if err != nil {
		log.WarnContext(ctx, "Error assigning session host to user", "Error", err)
		return nil, err
	}

	// Find desktop application group from host pool
	log.DebugContext(ctx, "Retrieving desktop application group", "HostPoolName", hpName)
	desktopApplicationGroup, err := avd.GetDesktopApplicationGroupFromHostpool(ctx, rgName, hpName)
	if err != nil {
		log.WarnContext(ctx, "Error retrieving desktop application group", "Error", err)
		return nil, err
	}

	// Determine workspace from desktop application group
	log.DebugContext(ctx, "Determining workspace from desktop application group")
	workspacePathSegments := strings.Split(*desktopApplicationGroup.Properties.WorkspaceArmPath, "/")
	workspaceName := workspacePathSegments[len(workspacePathSegments)-1]
	log.DebugContext(ctx, "Parsed workspace name", "WorkspaceName", workspaceName)

	workspace, err := avd.GetWorkspaceByName(ctx, rgName, workspaceName)
	if err != nil {
		log.WarnContext(ctx, "Error retrieving workspace", "Error", err)
		return nil, err
	}
	workspaceID := *workspace.Properties.ObjectID
	log.DebugContext(ctx, "Retrieved workspace ID", "WorkspaceID", workspaceID)

	// Find the resource ID of the desktop application
	log.DebugContext(ctx, "Retrieving desktop application", "DesktopApplicationGroup", *desktopApplicationGroup.Name)
	desktop, err := avd.getSingleDesktop(ctx, rgName, *desktopApplicationGroup.Name)
	if err != nil {
		log.WarnContext(ctx, "Error retrieving desktop application", "Error", err)
		return nil, err
	}
	resourceID := *desktop.Properties.ObjectID
	log.DebugContext(ctx, "Retrieved desktop application resource ID", "ResourceID", resourceID)

	// Generate connection URL
	log.DebugContext(ctx, "Generating Windows client URI", "avdUriVersion", avd.config.UriVersion, "UseMultiMon", avd.config.UseMultipleMonitors)
	connection := &models.VirtualMachineConnection{
		RemoteDesktopProvider: "AVD",
		URL:                   generateWindowsClientURI(workspaceID, resourceID, vm.UserID, avd.config.UriEnv, avd.config.UriVersion, toBool(avd.config.UseMultipleMonitors)),
	}

	vm.Connect = connection
	log.InfoContext(ctx, "PostRegister process completed successfully", "VM", vm.ID)

	return vm, nil
}

// To be completed when a VM is deleted.
// Removes the session host and cleans up empty host pools / app groups / workspaces
func (avd *AzureVirtualDesktopManager) Cleanup(ctx context.Context, vmID string) error {
	log := logging.GetLogger(ctx)
	rgName := avd.credentials.ResourceGroup

	hpFilter := avd.config.HostPoolNamePrefix
	log.InfoContext(ctx, "Starting AVD Cleanup process", "vmID", vmID, "resourceGroup", rgName, "hostPoolFilter", hpFilter)

	// acquire lock so we don't have concurrency issues with other threads deleting or creating host pools
	avd.stackMutex.Lock()
	defer avd.stackMutex.Unlock()
	log.DebugContext(ctx, "AVD cleanup - cleared AVD stack lock")

	// Step 1: Check existing host pools
	hostPools, err := avd.listHostPools(ctx, rgName, &hpFilter)
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
		sessionHostsPager := avd.sessionHostsClient.NewListPager(rgName, *hostPool.Name, nil)
		for sessionHostsPager.More() {
			page, err := sessionHostsPager.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("error listing session hosts in host pool %s: %w", *hostPool.Name, err)
			}

			for _, sessionHost := range page.Value {
				if sessionHost.Name != nil && strings.Contains(*sessionHost.Name, vmID) {
					// Extract sessionhostname from "hostpoolname/sessionhostname"
					sessionHostParts := strings.Split(*sessionHost.Name, "/")
					if len(sessionHostParts) != 2 {
						return fmt.Errorf("found session host with VM ID in name, but its name format is invalid. sessionHost.Name=%s. err=%v", *sessionHost.Name, err)
					}
					sessionHostName := sessionHostParts[1]

					log.InfoContext(ctx, "Found session host associated with VM; deleting", "sessionHost", sessionHostName, "hostPoolName", *hostPool.Name)
					_, err := avd.sessionHostsClient.Delete(ctx, rgName, *hostPool.Name, sessionHostName, nil)
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
		isEmpty, err := avd.isHostPoolEmpty(ctx, rgName, *hostPool.Name)
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
		err := avd.deleteStack(ctx, rgName, hostPoolNameToDelete)
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
	rgName := avd.credentials.ResourceGroup

	tags := map[string]*string{
		"stack_group_suffix": to.Ptr(suffix),
		"arkloud_created_by": to.Ptr("cloudy-azure"),
	}

	// Create host pool
	hostPool, err := avd.CreateHostPool(ctx, rgName, suffix, tags)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create host pool: %w", err)
	}

	// Create application group linked to the new host pool
	desktopAppGroup, err := avd.CreateApplicationGroup(ctx, rgName, suffix, tags)
	if err != nil {
		// TODO: delete resources during failed stack creation
		return nil, nil, nil, fmt.Errorf("failed to create application group: %w", err)
	}

	desktopApp, err := avd.renameDesktop(ctx, rgName, *desktopAppGroup.Name, suffix, avd.config.DesktopNamePrefix)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "error renaming desktopApp during stack creation")
	}
	_ = desktopApp

	// Create workspace linked to the desktop application group
	workspace, err := avd.CreateWorkspace(ctx, rgName, suffix, *desktopAppGroup.Name, tags)
	if err != nil {
		// TODO: delete resources during failed stack creation
		return nil, nil, nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	// Assign AVD user group to the new desktop application group (DAG)
	err = avd.AssignGroupToDesktopAppGroup(ctx, *desktopAppGroup.Name)
	if err != nil {
		// TODO: delete resources during failed stack creation
		return nil, nil, nil, fmt.Errorf("unable to assign AVD User group to Desktop Application Group [%s]", *desktopAppGroup.Name)
	}

	return hostPool, desktopAppGroup, workspace, nil
}

func toBool(input string) bool {
	output, err := strconv.ParseBool(input)
	if err != nil {
		return false
	}
	return output
}
