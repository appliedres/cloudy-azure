package vm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"

	cloudyazure "github.com/appliedres/cloudy-azure"
	cloudyvm "github.com/appliedres/cloudy/vm"
	avd "github.com/appliedres/cloudy-azure/avd"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"

	"github.com/pkg/errors"
)

const (
	MIN_WINDOWS_OS_DISK_SIZE = 200
)

type AzureVirtualMachineManager struct {
	credentials *cloudyazure.AzureCredentials
	config      *VirtualMachineManagerConfig

	vmClient     *armcompute.VirtualMachinesClient
	nicClient    *armnetwork.InterfacesClient
	diskClient   *armcompute.DisksClient
	subnetClient *armnetwork.SubnetsClient

	sizesClient *armcompute.ResourceSKUsClient
	usageClient *armcompute.UsageClient

	galleryClient *armcompute.SharedGalleryImageVersionsClient

	LogBody bool

	avdManager *avd.AzureVirtualDesktopManager
}

func NewAzureVirtualMachineManager(ctx context.Context, credentials *cloudyazure.AzureCredentials, 
		config *VirtualMachineManagerConfig, avdManager *avd.AzureVirtualDesktopManager) (cloudyvm.VirtualMachineManager, error) {

	vmm := &AzureVirtualMachineManager{
		credentials: credentials,
		config:      config,

		LogBody: false,

		avdManager: avdManager,
	}
	err := vmm.Configure(ctx)
	if err != nil {
		return nil, err
	}

	return vmm, nil
}

func (vmm *AzureVirtualMachineManager) Configure(ctx context.Context) error {
	credential, err := cloudyazure.NewAzureCredentials(vmm.credentials)
	if err != nil {
		return err
	}

	// Support using a separate resource group for the VNET / NIC / Subnet
	vnetAzCred := &cloudyazure.AzureCredentials{
		Type:           vmm.credentials.Type,
		Region:         vmm.credentials.Region,
		TenantID:       vmm.credentials.TenantID,
		ClientID:       vmm.credentials.ClientID,
		ClientSecret:   vmm.credentials.ClientSecret,
		ResourceGroup:  vmm.config.VnetResourceGroup, // only RG is changed
		SubscriptionID: vmm.credentials.SubscriptionID,
		Cloud:          vmm.credentials.Cloud,
	}
	VnetCredential, err := cloudyazure.NewAzureCredentials(vnetAzCred)
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

	nicClient, err := armnetwork.NewInterfacesClient(vmm.credentials.SubscriptionID, VnetCredential, options)
	if err != nil {
		return err
	}
	vmm.nicClient = nicClient

	diskClient, err := armcompute.NewDisksClient(vmm.credentials.SubscriptionID, credential, options)
	if err != nil {
		return err
	}
	vmm.diskClient = diskClient

	subnetClient, err := armnetwork.NewSubnetsClient(vmm.credentials.SubscriptionID, VnetCredential, options)
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

	return nil
}

func (vmm *AzureVirtualMachineManager) Start(ctx context.Context, vmName string) error {
	log := logging.GetLogger(ctx)

	poller, err := vmm.vmClient.BeginStart(ctx, vmm.credentials.ResourceGroup, vmName, &armcompute.VirtualMachinesClientBeginStartOptions{})
	if err != nil {
		return errors.Wrap(err, "VM Start")
	}

	_, err = cloudyazure.PollWrapper(ctx, poller, "VM Start")
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

	_, err = cloudyazure.PollWrapper(ctx, poller, "VM Stop")
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
		if cloudyazure.Is404(err) {
			log.InfoContext(ctx, "BeginDeallocate - VM not found")
			return nil
		}

		return errors.Wrap(err, "VM Deallocate")
	}

	_, err = cloudyazure.PollWrapper(ctx, poller, "VM Deallocate")
	if err != nil {
		return errors.Wrap(err, "VM Deallocate")
	}

	log.InfoContext(ctx, "VM Deallocate complete")

	return nil
}

func (vmm *AzureVirtualMachineManager) Update(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	return nil, nil
}

func UpdateCloudyVirtualMachine(vm *models.VirtualMachine, responseVirtualMachine armcompute.VirtualMachine) error {

	return nil
}

// ExecuteRemotePowershell executes a PowerShell script on a remote Azure virtual machine.
// It constructs a RunCommandInput for PowerShell execution, starts the execution, and polls for the result.
func (vmm *AzureVirtualMachineManager) ExecuteRemotePowershell(ctx context.Context, vmID string, script *string, timeout, pollInterval time.Duration) error {
	log := logging.GetLogger(ctx)

	log.DebugContext(ctx, "Constructing RunCommandInput for PowerShell execution")
	runCommandInput := armcompute.RunCommandInput{
		CommandID: to.Ptr("RunPowerShellScript"),
		Script: []*string{
			script,
		},
	}
	log.DebugContext(ctx, "Finished constructing RunCommandInput for PowerShell execution")

	log.InfoContext(ctx, "Executing remote PowerShell script")
	poller, err := vmm.vmClient.BeginRunCommand(ctx, vmm.credentials.ResourceGroup, vmID, runCommandInput, nil)
	if err != nil {
		log.ErrorContext(ctx, "Failed to execute remote PowerShell script", "error", err)
		return logging.LogAndWrapErr(ctx, log, err, "failed to execute remote PowerShell script")
	}

	log.DebugContext(ctx, "PowerShell command execution started successfully, polling for result")

	result, err := pollPowerShellExecution(ctx, poller, timeout, pollInterval)
	if err != nil {
		return err
	}

	return processPowerShellResult(ctx, result)
}

// pollPowerShellExecution polls the status of a PowerShell execution command until it completes or times out.
func pollPowerShellExecution(ctx context.Context, response *runtime.Poller[armcompute.VirtualMachinesClientRunCommandResponse], timeout time.Duration, pollInterval time.Duration) (armcompute.RunCommandResult, error) {
	log := logging.GetLogger(ctx)

	startTime := time.Now()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	log.DebugContext(ctx, fmt.Sprintf("PowerShell execution polling initialized. Timeout: %d min %d sec", int(timeout.Minutes()), int(timeout.Seconds())%60))

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done(): // Execution timeout reached
			log.WarnContext(ctx, "PowerShell execution timed out", "elapsed", time.Since(startTime))

			// Attempt to retrieve partial output before exiting
			if response.Done() {
				finalResult, err := response.Result(ctx)
				if err != nil {
					log.ErrorContext(ctx, "Failed to retrieve partial RunCommand result", "error", err)
					return armcompute.RunCommandResult{}, logging.LogAndWrapErr(ctx, log, err, "failed to retrieve partial RunCommand result")
				}
				return finalResult.RunCommandResult, nil
			}

			return armcompute.RunCommandResult{}, fmt.Errorf("PowerShell execution timed out after %v", timeout)

		case <-ticker.C:
			elapsed := time.Since(startTime)
			remaining := timeout - elapsed
			log.InfoContext(ctx, fmt.Sprintf(
				"Polling PowerShell execution status. Elapsed: %d min %d sec, Timeout remaining: %d min %d sec",
				int(elapsed.Minutes()), int(elapsed.Seconds())%60, int(remaining.Minutes()), int(remaining.Seconds())%60,
			))

			_, err := response.Poll(ctx)
			if err != nil {
				log.ErrorContext(ctx, "Failed to retrieve RunCommand result", "error", err)
				return armcompute.RunCommandResult{}, logging.LogAndWrapErr(ctx, log, err, "failed to retrieve RunCommand result")
			}

			if response.Done() {
				finalResult, err := response.Result(ctx)
				if err != nil {
					log.ErrorContext(ctx, "Failed to retrieve final RunCommand result", "error", err)
					return armcompute.RunCommandResult{}, logging.LogAndWrapErr(ctx, log, err, "failed to retrieve final RunCommand result")
				}
				log.InfoContext(ctx, "Powershell execution completed", "elapsed", time.Since(startTime))
				return finalResult.RunCommandResult, nil
			}
		}
	}
}

// processPowerShellResult processes and logs the output of a PowerShell script execution.
func processPowerShellResult(ctx context.Context, result armcompute.RunCommandResult) error {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "PowerShell execution completed, processing result")

	// Output the command's result line by line
	if len(result.Value) > 0 {
		for _, output := range result.Value {
			if output.Message != nil {
				message := *output.Message
				lines := strings.Split(message, "\n")

				for _, line := range lines {
					trimmedLine := strings.TrimSpace(line)
					if trimmedLine == "" {
						continue // Skip empty lines
					}

					safeLogContent := fmt.Sprintf("[PowerShell Output]: %s", trimmedLine)

					if strings.Contains(trimmedLine, "ERROR") {
						err := fmt.Errorf("PowerShell response contains an error: %s", trimmedLine)
						log.ErrorContext(ctx, safeLogContent)
						return logging.LogAndWrapErr(ctx, log, err, "PowerShell script error detected")
					}

					log.InfoContext(ctx, safeLogContent)
				}
			}
		}
	} else {
		log.WarnContext(ctx, "No output returned from the PowerShell execution")
	}

	log.DebugContext(ctx, "RunPowerShell function completed successfully")
	return nil
}
