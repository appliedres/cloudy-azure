package vm

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v5"

	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"

	"github.com/pkg/errors"
)

const (
	MIN_WINDOWS_OS_DISK_SIZE = 200
)

type AzureVirtualMachineManager struct {
	name string

	Credentials *cloudyazure.AzureCredentials
	Config      *VirtualMachineManagerConfig

	vmClient     *armcompute.VirtualMachinesClient
	nicClient    *armnetwork.InterfacesClient
	diskClient   *armcompute.DisksClient
	subnetClient *armnetwork.SubnetsClient

	sizesClient *armcompute.ResourceSKUsClient
	usageClient *armcompute.UsageClient

	galleryClient *armcompute.SharedGalleryImageVersionsClient

	LogBody bool
}

func NewAzureVirtualMachineManager(
	ctx context.Context, name string,
	credentials *cloudyazure.AzureCredentials,
	config *VirtualMachineManagerConfig,
) (*AzureVirtualMachineManager, error) {

	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "NewAzureVirtualMachineManager started")
	defer log.DebugContext(ctx, "NewAzureVirtualMachineManager complete")

	vmm := &AzureVirtualMachineManager{
		name: name,

		Credentials: credentials,
		Config:      config,

		LogBody: false,
	}
	err := vmm.Configure(ctx)
	if err != nil {
		return nil, err
	}

	return vmm, nil
}

func (vmm *AzureVirtualMachineManager) Configure(ctx context.Context) error {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "VM Manager Configure started")
	defer log.DebugContext(ctx, "VM Manager Configure complete")

	credential, err := cloudyazure.NewAzureCredentials(vmm.Credentials)
	if err != nil {
		return err
	}

	// Support using a separate resource group for the VNET / NIC / Subnet
	vnetAzCred := &cloudyazure.AzureCredentials{
		Type:           vmm.Credentials.Type,
		Region:         vmm.Credentials.Region,
		TenantID:       vmm.Credentials.TenantID,
		ClientID:       vmm.Credentials.ClientID,
		ClientSecret:   vmm.Credentials.ClientSecret,
		ResourceGroup:  vmm.Config.VnetResourceGroup, // only RG is changed
		SubscriptionID: vmm.Credentials.SubscriptionID,
		Cloud:          vmm.Credentials.Cloud,
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

	vmClient, err := armcompute.NewVirtualMachinesClient(vmm.Credentials.SubscriptionID, credential, options)
	if err != nil {
		return err
	}
	vmm.vmClient = vmClient

	nicClient, err := armnetwork.NewInterfacesClient(vmm.Credentials.SubscriptionID, VnetCredential, options)
	if err != nil {
		return err
	}
	vmm.nicClient = nicClient

	diskClient, err := armcompute.NewDisksClient(vmm.Credentials.SubscriptionID, credential, options)
	if err != nil {
		return err
	}
	vmm.diskClient = diskClient

	subnetClient, err := armnetwork.NewSubnetsClient(vmm.Credentials.SubscriptionID, VnetCredential, options)
	if err != nil {
		return err
	}
	vmm.subnetClient = subnetClient

	sizesClient, err := armcompute.NewResourceSKUsClient(vmm.Credentials.SubscriptionID, credential, options)
	if err != nil {
		return err
	}
	vmm.sizesClient = sizesClient

	galleryClient, err := armcompute.NewSharedGalleryImageVersionsClient(vmm.Credentials.SubscriptionID, credential, options)
	if err != nil {
		return err
	}
	vmm.galleryClient = galleryClient

	usageClient, err := armcompute.NewUsageClient(vmm.Credentials.SubscriptionID, credential, options)
	if err != nil {
		return err
	}
	vmm.usageClient = usageClient

	return nil
}

func (vmm *AzureVirtualMachineManager) StartVirtualMachine(ctx context.Context, vmName string) error {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "VM Start")
	defer log.DebugContext(ctx, "VM Start complete")

	poller, err := vmm.vmClient.BeginStart(ctx, vmm.Credentials.ResourceGroup, vmName, &armcompute.VirtualMachinesClientBeginStartOptions{})
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

func (vmm *AzureVirtualMachineManager) StopVirtualMachine(ctx context.Context, vmName string) error {
	return vmm.deallocateVirtualMachine(ctx, vmName)
}

// Warning: using this method will stop the VM, but not deallocate it, causing it to continue to incur costs.
func (vmm *AzureVirtualMachineManager) powerOffVirtualMachine(ctx context.Context, vmName string) error {
	log := logging.GetLogger(ctx)

	poller, err := vmm.vmClient.BeginPowerOff(ctx, vmm.Credentials.ResourceGroup, vmName, &armcompute.VirtualMachinesClientBeginPowerOffOptions{})
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

func (vmm *AzureVirtualMachineManager) deallocateVirtualMachine(ctx context.Context, vmName string) error {
	log := logging.GetLogger(ctx)

	poller, err := vmm.vmClient.BeginDeallocate(ctx, vmm.Credentials.ResourceGroup, vmName, &armcompute.VirtualMachinesClientBeginDeallocateOptions{})
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

func (vmm *AzureVirtualMachineManager) UpdateVirtualMachine(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	return nil, nil
}

func UpdateCloudyVirtualMachine(vm *models.VirtualMachine, responseVirtualMachine armcompute.VirtualMachine) error {

	return nil
}

// ExecuteRemotePowerShell executes a PowerShell script on a remote Azure virtual machine.
// It uses the RunCommand API with the "RunPowerShellScript" command.
func (vmm *AzureVirtualMachineManager) ExecuteRemotePowershell(ctx context.Context, vmID string, script *string, timeout, pollInterval time.Duration) error {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "Beginning Powershell script execution")
	defer log.DebugContext(ctx, "Powershell script execution completed")

	return vmm.executeRemoteCommand(ctx, vmID, "RunPowerShellScript", "PowerShell", script, timeout, pollInterval)
}

// ExecuteRemoteShellScript executes a shell script on a remote Azure virtual machine.
// It uses the RunCommand API with the "RunShellScript" command.
func (vmm *AzureVirtualMachineManager) ExecuteRemoteShellScript(ctx context.Context, vmID string, script *string, timeout, pollInterval time.Duration) error {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "Beginning Shell script execution")
	defer log.DebugContext(ctx, "Shell script execution completed")

	return vmm.executeRemoteCommand(ctx, vmID, "RunShellScript", "Shell Script", script, timeout, pollInterval)
}

// executeRemoteCommand encapsulates the common logic for executing a remote command.
// The commandID and label (e.g., "PowerShell" or "Shell Script") differentiate the two types.
func (vmm *AzureVirtualMachineManager) executeRemoteCommand(ctx context.Context, vmID, commandID, label string, script *string, timeout, pollInterval time.Duration) error {
	log := logging.GetLogger(ctx)

	log.DebugContext(ctx, fmt.Sprintf("Constructing RunCommandInput for %s execution", label))
	runCommandInput := armcompute.RunCommandInput{
		CommandID: to.Ptr(commandID),
		Script: []*string{
			script,
		},
	}
	log.DebugContext(ctx, fmt.Sprintf("Finished constructing RunCommandInput for %s execution", label))

	log.InfoContext(ctx, fmt.Sprintf("Executing remote %s script", label))
	poller, err := vmm.vmClient.BeginRunCommand(ctx, vmm.Credentials.ResourceGroup, vmID, runCommandInput, nil)
	if err != nil {
		log.ErrorContext(ctx, fmt.Sprintf("Failed to execute remote %s script", label), "error", err)
		return logging.LogAndWrapErr(ctx, log, err, fmt.Sprintf("failed to execute remote %s script", label))
	}

	log.DebugContext(ctx, fmt.Sprintf("%s command execution started successfully, polling for result", label))
	result, err := pollCommandExecution(ctx, poller, timeout, pollInterval, label)
	if err != nil {
		return err
	}

	return processCommandResult(ctx, result, label)
}

// pollCommandExecution polls the status of a remote command execution until it completes or times out.
func pollCommandExecution(ctx context.Context, response *runtime.Poller[armcompute.VirtualMachinesClientRunCommandResponse], timeout, pollInterval time.Duration, label string) (armcompute.RunCommandResult, error) {
	log := logging.GetLogger(ctx)

	startTime := time.Now()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	log.DebugContext(ctx, fmt.Sprintf("%s execution polling initialized. Timeout: %d min %d sec", label, int(timeout.Minutes()), int(timeout.Seconds())%60))

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.WarnContext(ctx, fmt.Sprintf("%s execution timed out", label), "elapsed", time.Since(startTime))
			// Attempt to retrieve partial output before exiting
			if response.Done() {
				finalResult, err := response.Result(ctx)
				if err != nil {
					log.ErrorContext(ctx, "Failed to retrieve partial RunCommand result", "error", err)
					return armcompute.RunCommandResult{}, logging.LogAndWrapErr(ctx, log, err, "failed to retrieve partial RunCommand result")
				}
				return finalResult.RunCommandResult, nil
			}
			return armcompute.RunCommandResult{}, fmt.Errorf("%s execution timed out after %v", label, timeout)
		case <-ticker.C:
			elapsed := time.Since(startTime)
			remaining := timeout - elapsed
			log.InfoContext(ctx, fmt.Sprintf(
				"Polling %s execution status. Elapsed: %d min %d sec, Timeout remaining: %d min %d sec",
				label, int(elapsed.Minutes()), int(elapsed.Seconds())%60, int(remaining.Minutes()), int(remaining.Seconds())%60,
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
				log.InfoContext(ctx, fmt.Sprintf("%s execution completed", label), "elapsed", time.Since(startTime))
				return finalResult.RunCommandResult, nil
			}
		}
	}
}

// processCommandResult processes and logs the output of a remote command execution.
func processCommandResult(ctx context.Context, result armcompute.RunCommandResult, label string) error {
	var controlChars = regexp.MustCompile(`[\x00-\x1F\x7F]`)

	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, fmt.Sprintf("%s execution completed, processing result", label))

	if len(result.Value) > 0 {
		for _, output := range result.Value {
			if output.Message != nil {
				message := *output.Message
				lines := strings.Split(message, "\n")
				for _, line := range lines {
					// trim whitespaces
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}

					// remove control characters
					line = controlChars.ReplaceAllString(line, "")

					// ensure valid UTF-8 encoding
					// This is a safeguard, as Azure should return valid UTF-8, but we handle it just in case.
					if !utf8.ValidString(line) {
						line = strings.ToValidUTF8(line, "")
					}

					// add labeling
					safeLogContent := fmt.Sprintf("[%s Output]: %s", label, line)
					if strings.Contains(strings.ToLower(line), "error") {
						err := fmt.Errorf("%s response contains an error: %s", label, line)
						log.ErrorContext(ctx, safeLogContent)
						return logging.LogAndWrapErr(ctx, log, err, fmt.Sprintf("%s script error detected", label))
					}

					log.InfoContext(ctx, safeLogContent)
				}
			}
		}
	} else {
		log.WarnContext(ctx, fmt.Sprintf("No output returned from the %s execution", label))
	}

	log.DebugContext(ctx, fmt.Sprintf("Run%s function completed successfully", label))
	return nil
}
