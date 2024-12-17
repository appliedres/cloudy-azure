package cloudyazure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	cloudyvm "github.com/appliedres/cloudy/vm"
	"github.com/pkg/errors"
)

const (
	MIN_WINDOWS_OS_DISK_SIZE = 200
)

type AzureVirtualMachineManager struct {
	credentials *AzureCredentials
	config      *VirtualMachineManagerConfig

	vmClient     *armcompute.VirtualMachinesClient
	nicClient    *armnetwork.InterfacesClient
	diskClient   *armcompute.DisksClient
	subnetClient *armnetwork.SubnetsClient

	sizesClient  *armcompute.ResourceSKUsClient
	usageClient *armcompute.UsageClient

	galleryClient *armcompute.SharedGalleryImageVersionsClient

	avdManager *AzureVirtualDesktopManager

	LogBody bool
}

func NewAzureVirtualMachineManager(ctx context.Context, credentials *AzureCredentials, config *VirtualMachineManagerConfig) (cloudyvm.VirtualMachineManager, error) {

	vmm := &AzureVirtualMachineManager{
		credentials: credentials,
		config:      config,

		LogBody: false,
	}
	err := vmm.Configure(ctx)
	if err != nil {
		return nil, err
	}

	return vmm, nil
}

func (vmm *AzureVirtualMachineManager) Configure(ctx context.Context) error {
	credential, err := NewAzureCredentials(vmm.credentials)
	if err != nil {
		return err
	}

	options := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
			Logging: policy.LogOptions{
				IncludeBody: vmm.LogBody,
			},
		},
	}

	vmClient, err := armcompute.NewVirtualMachinesClient(vmm.credentials.SubscriptionID, credential, options)
	if err != nil {
		return err
	}
	vmm.vmClient = vmClient

	nicClient, err := armnetwork.NewInterfacesClient(vmm.credentials.SubscriptionID, credential, options)
	if err != nil {
		return err
	}
	vmm.nicClient = nicClient

	diskClient, err := armcompute.NewDisksClient(vmm.credentials.SubscriptionID, credential, options)
	if err != nil {
		return err
	}
	vmm.diskClient = diskClient

	subnetClient, err := armnetwork.NewSubnetsClient(vmm.credentials.SubscriptionID, credential, options)
	if err != nil {
		return err
	}
	vmm.subnetClient = subnetClient

	sizesClient, err := armcompute.NewResourceSKUsClient(vmm.credentials.SubscriptionID, credential, options)
	if err != nil {
		return err
	}
	vmm.sizesClient = sizesClient

	galleryClient, err := armcompute.NewSharedGalleryImageVersionsClient(vmm.credentials.SubscriptionID, credential, options)
	if err != nil {
		return err
	}
	vmm.galleryClient = galleryClient

	usageClient, err := armcompute.NewUsageClient(vmm.credentials.SubscriptionID, credential, options)
	if err != nil {
		return err
	}
	vmm.usageClient = usageClient

	avdManager, err := NewAzureVirtualDesktopManager(ctx, vmm.credentials, &vmm.config.AvdConfig)
	if err != nil {
		return err
	}
	vmm.avdManager = avdManager

	return nil
}

func (vmm *AzureVirtualMachineManager) Start(ctx context.Context, vmName string) error {
	log := logging.GetLogger(ctx)

	poller, err := vmm.vmClient.BeginStart(ctx, vmm.credentials.ResourceGroup, vmName, &armcompute.VirtualMachinesClientBeginStartOptions{})
	if err != nil {
		return errors.Wrap(err, "VM Start")
	}

	_, err = pollWrapper(ctx, poller, "VM Start")
	if err != nil {
		return errors.Wrap(err, "VM Start")
	}

	log.InfoContext(ctx, "VM Stop complete")

	return nil
}

func (vmm *AzureVirtualMachineManager) Stop(ctx context.Context, vmName string) error {
	log := logging.GetLogger(ctx)

	poller, err := vmm.vmClient.BeginPowerOff(ctx, vmm.credentials.ResourceGroup, vmName, &armcompute.VirtualMachinesClientBeginPowerOffOptions{})
	if err != nil {
		return errors.Wrap(err, "VM Stop")
	}

	_, err = pollWrapper(ctx, poller, "VM Stop")
	if err != nil {
		return errors.Wrap(err, "VM Stop")
	}

	log.InfoContext(ctx, "VM Stop complete")

	return nil
}

func (vmm *AzureVirtualMachineManager) Deallocate(ctx context.Context, vmName string) error {
	log := logging.GetLogger(ctx)

	poller, err := vmm.vmClient.BeginDeallocate(ctx, vmm.credentials.ResourceGroup, vmName, &armcompute.VirtualMachinesClientBeginDeallocateOptions{})
	if err != nil {
		if is404(err) {
			log.InfoContext(ctx, "BeginDeallocate - VM not found")
			return nil
		}

		return errors.Wrap(err, "VM Deallocate")
	}

	_, err = pollWrapper(ctx, poller, "VM Deallocate")
	if err != nil {
		return errors.Wrap(err, "VM Deallocate")
	}

	log.InfoContext(ctx, "VM Deallocate complete")

	return nil
}

func (vmm *AzureVirtualMachineManager) Update(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	return nil, nil
}

func (vmm *AzureVirtualMachineManager) AvdRegister(ctx context.Context, vm *models.VirtualMachine, avdRgName, hpName, upn, domainName, domainUsername, domainPassword string) error {
	var err error
	
	regToken, err := vmm.avdManager.PreRegister(ctx, avdRgName, hpName)
	if err != nil {
		return errors.Wrap(err, "AVD PreRegister")
	}

	err = vmm.runAvdRegistrationScript(ctx, vm, *regToken, domainName, domainUsername, domainPassword)
	if err != nil {
		return errors.Wrap(err, "AVD Register Script")
	}

	err = vmm.avdManager.PostRegister(ctx, avdRgName, hpName, vm.ID, upn)
	if err != nil {
		return errors.Wrap(err, "AVD PostRegister")
	}

	return nil
}

// Given a vmName, generates a VirtualMachineConnection from AVD, which includes the connection URL
func (vmm *AzureVirtualMachineManager) Connect(ctx context.Context, vmID string) (*models.VirtualMachineConnection, error) {
	rgName := "arkloud-avd-testing-usva"  // TODO: handle separate RG for AVD
	hostPoolName := "vulcanpp-AVD-HP-0"  // TODO: pass in host pool name from avd config

	sessionHost, err := vmm.avdManager.FindSessionHostForVM(ctx, rgName, hostPoolName, vmID)
	if err != nil {
		return nil, errors.Wrap(err, "Connect failed, error finding session host")
	}
	if sessionHost == nil {
		return nil, fmt.Errorf("Could not find a session host for VM [%s]", vmID)
	}

	// sessionHost.Name is in the format "hostpoolName/sessionHostName", so we need to split it
	parts := strings.SplitN(*sessionHost.Name, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("Could not split sessionHost.Name: %s", *sessionHost.Name)
	}
	sessionHostName := parts[1]
	
	connection, err := vmm.avdManager.GenerateConnectionURL(ctx, hostPoolName, sessionHostName, vmID)
	if err != nil {
		return nil, errors.Wrap(err, "AVD Host Pool list")
	}

	return connection, nil
}

func (vmm *AzureVirtualMachineManager) runAvdRegistrationScript(ctx context.Context, vm *models.VirtualMachine, registrationKey, domainName, domainUsername, domainPassword string) error {	
	rgName := vmm.credentials.ResourceGroup
	vmName := vm.ID
	osType := vm.Template.OperatingSystem

	if osType != "windows" {
		return errors.New("unsupported OS type; only Windows is supported for AVD registration")
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
	script := fmt.Sprintf(scriptTemplate, registrationKey, domainName, domainUsername, domainPassword)

	// Define RunCommandInput
	runCommandInput := armcompute.RunCommandInput{
		CommandID: to.Ptr("RunPowerShellScript"),
		Script: []*string{
			to.Ptr(script),
		},
	}

	// Execute the script
	response, err := vmm.vmClient.BeginRunCommand(ctx, rgName, vmName, runCommandInput, nil)
	if err != nil {
		return errors.Wrap(err, "failed to execute AVD registration script")
	}

	// Poll until the command completes
	result, err := response.PollUntilDone(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve RunCommand result")
	}

	// Output the command's result
	if result.Value != nil && len(result.Value) > 0 {
		for _, output := range result.Value {
			if output.Message != nil {
				fmt.Printf("Command Output: %s\n", *output.Message)
			}
		}
	} else {
		fmt.Println("No output returned from the command.")
	}

	return nil
}



func UpdateCloudyVirtualMachine(vm *models.VirtualMachine, responseVirtualMachine armcompute.VirtualMachine) error {

	return nil
}
