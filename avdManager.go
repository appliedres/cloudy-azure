package cloudyazure

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	"github.com/pkg/errors"
)

const (
	// TODO: Make these an AVD config item, stored in AVD manager
	prefixBase = "VULCAN-AVD"
	hostPoolNamePrefix = prefixBase+"-HP-"
	workspaceNamePrefix = prefixBase+"-WS-"
	appGroupNamePrefix = prefixBase+"-DAG-"
	desktopApplicationUserRoleID = "1d18fff3-a72a-46b5-b4a9-0b38a3cd7e63"  // https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/compute#desktop-virtualization-user
	uriEnv = "usgov"
)

type AzureVirtualDesktopManager struct {
	credentials 	*AzureCredentials
	config      	*AzureVirtualDesktopConfig
	
	workspacesClient      *armdesktopvirtualization.WorkspacesClient
	hostPoolsClient       *armdesktopvirtualization.HostPoolsClient
	sessionHostsClient    *armdesktopvirtualization.SessionHostsClient
	userSessionsClient    *armdesktopvirtualization.UserSessionsClient
	appGroupsClient       *armdesktopvirtualization.ApplicationGroupsClient
	appsClient			  *armdesktopvirtualization.ApplicationsClient
	desktopsClient		  *armdesktopvirtualization.DesktopsClient

	roleAssignmentsClient *armauthorization.RoleAssignmentsClient
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

	baseOptions := arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
		
	}

	avdOptions := baseOptions
	avdOptions.APIVersion = "2023-09-05"  // Important! Latest AVD API version is not supported in Azure Govt
	clientFactory, err := armdesktopvirtualization.NewClientFactory(avd.credentials.SubscriptionID, cred, &avdOptions)
	if err != nil {
		return err
	}

	avd.workspacesClient = 	clientFactory.NewWorkspacesClient()
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

	return nil
}


// Prior to VM registration, this process generates a token for a given host pool.
// This token will later be used in the registration process to join the VM to the host pool
// The user is also assigned to the related desktop application group.
func (avd *AzureVirtualDesktopManager) PreRegister(ctx context.Context, vm *models.VirtualMachine) (hostPoolName, token *string, err error) {
	log := logging.GetLogger(ctx)
	rgName := avd.credentials.ResourceGroup
	
	// Step 1: Check existing host pools
	hpFilter := hostPoolNamePrefix
	hostPools, err := avd.listHostPools(ctx, rgName, &hpFilter)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve host pools: %w", err)
	}

	// TODO: check that host pool is valid, with necessary resources (WS, DAG). It may have previously failed creation

	// Check if the user can be assigned to any existing host pool
	var targetHostPool *armdesktopvirtualization.HostPool
	var hostPoolSuffixes []string
	for _, pool := range hostPools {
		canAssign, err := avd.CanAssignUserToHostPool(ctx, rgName, *pool.Name, vm.UserID)
		if err != nil {
			return nil, nil, fmt.Errorf("Error checking assignments in host pool")
		}
		if canAssign {
			targetHostPool = pool
			break
		}
		hostPoolSuffixes = append(hostPoolSuffixes, strings.TrimPrefix(*pool.Name, hostPoolNamePrefix))
	}

	// TODO: tag all resources

	// Step 2: If no suitable host pool exists, create a new one
	if targetHostPool == nil {

		nameSuffix, err := GenerateNextName(hostPoolSuffixes, 2)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate new host pool name: %w", err)
		}

		log.InfoContext(ctx, "AVD: Creating new host pool, workspace, application group", "VM", vm.ID, "suffix", nameSuffix)

		// Create host pool
		targetHostPool, err = avd.CreateHostPool(ctx, rgName, nameSuffix)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create host pool: %w", err)
		}

		// Create application group linted to the new host pool
		desktopAppGroup, err := avd.CreateApplicationGroup(ctx, rgName, nameSuffix)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create application group: %w", err)
		}

		// Create workspace linked to the desktop application group
		workspace, err := avd.CreateWorkspace(ctx, rgName, nameSuffix, *desktopAppGroup.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create workspace: %w", err)
		}
		_ = workspace

		// Assign AVD user group to the new desktop application group (DAG)
		// FIXME: This is broken. Group is not being added as an assignment.
		err = avd.AssignGroupToDesktopAppGroup(ctx, *desktopAppGroup.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to assign AVD User group to Desktop Application Group [%s]", *desktopAppGroup.Name)
		}
	}

	// TODO: handle existing host pools with expired registration keys. need to call update host pool with new exp date

	// Step 3: Retrieve registration token
	token, err = avd.RetrieveRegistrationToken(ctx, rgName, *targetHostPool.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve registration token: %w", err)
	}

	return targetHostPool.Name, token, nil
}

func (avd *AzureVirtualDesktopManager) GetRegistrationScript(ctx context.Context, vm *models.VirtualMachine, registrationToken string) (*string, error) {	
	osType := vm.Template.OperatingSystem
	if osType != "windows" {
		return nil, errors.New("unsupported OS type; only Windows is supported for AVD registration")
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

# Check if the machine is already in the domain
$computerDomain = (Get-WmiObject -Class Win32_ComputerSystem).Domain
if ($computerDomain -eq $DOMAIN_NAME) {
    Write-Host "Machine is already part of the domain: $DOMAIN_NAME"
} else {
    # Join the domain
    try {
        Write-Host "Attempting to join the domain: $DOMAIN_NAME"
        $securePassword = ConvertTo-SecureString -String $DOMAIN_PASSWORD -AsPlainText -Force
        $credential = New-Object System.Management.Automation.PSCredential ($DOMAIN_USERNAME, $securePassword)
        Add-Computer -DomainName $DOMAIN_NAME -Credential $credential -Force -Verbose
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
	script := fmt.Sprintf(scriptTemplate, registrationToken, avd.config.DomainName, avd.config.DomainUser, avd.config.DomainPass)

	return &script, nil
}

// After registering, the user must then be assigned to the new session host
func (avd *AzureVirtualDesktopManager) PostRegister(ctx context.Context, vm *models.VirtualMachine, hpName string) (*models.VirtualMachine, error) {
	rgName := avd.credentials.ResourceGroup
	
	sessionHost, err := avd.WaitForSessionHost(ctx, rgName, hpName, vm.ID, 10*time.Minute)
	if err != nil {
		return nil, errors.Wrap(err, "Waiting for session host to be ready")
	}

	// sessionHost.Name is in the format "hostpoolName/sessionHostName", so we need to split it
	parts := strings.SplitN(*sessionHost.Name, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("Could not split sessionHost.Name: %s", *sessionHost.Name)
	}
	sessionHostName := parts[1]
	
	err = avd.AssignSessionHost(ctx, rgName, hpName, sessionHostName, vm.UserID)
	if err != nil {
		return nil, err
	}

	// find desktop application group from host pool
	desktopApplicationGroup, err := avd.GetDesktopApplicationGroupFromHostpool(ctx, rgName, hpName)
	if err != nil {
		return nil, err
	}

	// determine workspace from desktop application group
	workspacePathSegments := strings.Split(*desktopApplicationGroup.Properties.WorkspaceArmPath, "/")
	workspaceName := workspacePathSegments[len(workspacePathSegments)-1]

	workspace, err := avd.GetWorkspaceByName(ctx, rgName, workspaceName)
	if err != nil {
		return nil, err
	}
	workspaceID := *workspace.Properties.ObjectID

	// find the resource id of the desktop application
	desktop, err := avd.getSingleDesktop(ctx, rgName, *desktopApplicationGroup.Name)
	if err != nil {
		return nil, err
	}
	resourceID := *desktop.Properties.ObjectID

	version := "0"
	useMultiMon := false  // TODO: add support for multi monitor to endpoint
	
	connection := &models.VirtualMachineConnection{
		RemoteDesktopProvider: "AVD",
		URL:                   generateWindowsClientURI(workspaceID, resourceID, vm.UserID, uriEnv, version, useMultiMon),
	}

	vm.Connect = connection

	return vm, nil
}
