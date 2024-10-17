package cloudyazure

import (
	"context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
)

// func NewVMClient(ctx context.Context, config *AzureVMControllerConfig) (*armcompute.VirtualMachinesClient, error) {
// 	cred, err := GetAzureClientSecretCredential(config.AzureCredentials)
// 	if err != nil {
// 		return nil, cloudy.Error(ctx, "Authentication failure: %+v", err)
// 	}

// 	client, err := armcompute.NewVirtualMachinesClient(config.SubscriptionID, cred,
// 		&arm.ClientOptions{
// 			ClientOptions: policy.ClientOptions{
// 				Cloud: cloud.AzureGovernment,
// 				Logging: policy.LogOptions{
// 					IncludeBody: config.LogBody,
// 				},
// 			},
// 		})

// 	return client, err
// }

// func NewUsageClient(ctx context.Context, config *AzureVMControllerConfig) (*armcompute.UsageClient, error) {
// 	cred, err := GetAzureClientSecretCredential(config.AzureCredentials)
// 	if err != nil {
// 		return nil, cloudy.Error(ctx, "Authentication failure: %+v", err)
// 	}

// 	client, err := armcompute.NewUsageClient(config.SubscriptionID, cred,
// 		&arm.ClientOptions{
// 			ClientOptions: policy.ClientOptions{
// 				Cloud: cloud.AzureGovernment,
// 			},
// 		})

// 	return client, err
// }

// func getVMClient(ctx context.Context) (*armcompute.VirtualMachinesClient, error) {
// 	if ctx == nil {
// 		ctx = cloudy.StartContext()
// 	}

// 	tenantID := cloudy.DefaultEnvironment.Get("AZURE_COMPUTE_TENANT_ID")
// 	clientID := cloudy.DefaultEnvironment.Get("AZURE_COMPUTE_CLIENT_ID")
// 	clientSecret := cloudy.DefaultEnvironment.Get("AZURE_COMPUTE_CLIENT_SECRET")
// 	subscriptionID := cloudy.DefaultEnvironment.Get("AZURE_COMPUTE_SUBSCRIPTION_ID")

// 	azConfig := AzureCredentials{
// 		TenantID:     tenantID,
// 		ClientID:     clientID,
// 		ClientSecret: clientSecret,
// 	}

// 	cred, err := GetAzureClientSecretCredential(azConfig)
// 	// cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret,
// 	// 	&azidentity.ClientSecretCredentialOptions{AuthorityHost: azidentity.AzureGovernment})

// 	if err != nil {
// 		_ = cloudy.Error(ctx, "Authentication failure: %+v", err)
// 	}

// 	client, err := armcompute.NewVirtualMachinesClient(subscriptionID, cred,
// 		&arm.ClientOptions{
// 			ClientOptions: policy.ClientOptions{
// 				Cloud: cloud.AzureGovernment,
// 			},
// 		})

// 	return client, err
// }

// func VmList(ctx context.Context, vmClient *armcompute.VirtualMachinesClient, rg string) ([]*cloudyvm.VirtualMachineStatus, error) {
// 	var err error

// 	if ctx == nil {
// 		ctx = cloudy.StartContext()
// 	}

// 	var returnList []*cloudyvm.VirtualMachineStatus
// 	statusOnly := "true"

// 	pager := vmClient.NewListAllPager(&armcompute.VirtualMachinesClientListAllOptions{
// 		// Filter:     &filter,
// 		StatusOnly: &statusOnly,
// 	})
// 	for pager.More() {
// 		resp, err := pager.NextPage(ctx)
// 		if err != nil {
// 			_ = cloudy.Error(ctx, "VM Client List Error %s", err)
// 			return returnList, err
// 		}
// 		for _, vm := range resp.Value {
// 			// log.Printf("name: %s", *vm.Name)

// 			// var vmStatus *cloudyvm.VirtualMachineStatus
// 			// vmStatus, err = VmStatus(ctx, vmClient, *vm.Name, rg)

// 			vmStatus := &cloudyvm.VirtualMachineStatus{}
// 			vmStatus.Name = *vm.Name
// 			vmStatus.ProvisioningState = *vm.Properties.ProvisioningState
// 			vmStatus.LongID = *vm.ID
// 			vmStatus.ID = *vm.Properties.VMID

// 			for _, status := range vm.Properties.InstanceView.Statuses {
// 				if strings.Contains(*status.Code, "PowerState") {
// 					parts := strings.Split(*status.Code, "/")

// 					vmStatus.PowerState = parts[1]
// 				} else if strings.Contains(*status.Code, "ProvisioningState") && status.Time != nil {
// 					vmStatus.ProvisioningTime = *status.Time
// 				}
// 			}

// 			myRg := ExtractResourceGroupFromID(ctx, vmStatus.LongID)
// 			if strings.EqualFold(rg, myRg) {
// 				returnList = append(returnList, vmStatus)
// 			}

// 		}
// 	}

// 	return returnList, err
// }

func ExtractResourceGroupFromID(ctx context.Context, id string) string {
	parts := strings.Split(id, "/")
	if len(parts) >= 4 {
		return parts[4]
	}
	return ""
}

// func VmStatus(ctx context.Context, vmClient *armcompute.VirtualMachinesClient, vmName string, resourceGroup string) (*cloudyvm.VirtualMachineStatus, error) {
// 	if ctx == nil {
// 		ctx = cloudy.StartContext()
// 	}

// 	cloudy.Info(ctx, "VmStatus: %s (%s)", vmName, resourceGroup)

// 	var err error

// 	if vmClient == nil {
// 		// vmClient, err = getVMClient(ctx)

// 		if err != nil {
// 			return nil, err
// 		}
// 	}

// 	instanceType := armcompute.InstanceViewTypesInstanceView
// 	resp, err := vmClient.Get(context.Background(), resourceGroup, vmName,
// 		&armcompute.VirtualMachinesClientGetOptions{
// 			Expand: &instanceType,
// 		})

// 	if err != nil {
// 		if is404(err) {
// 			cloudy.Info(ctx, "[%s] VmStatus StatusNotFound", vmName)
// 			return nil, nil
// 		} else {
// 			_ = cloudy.Error(ctx, "[%s] VmStatus failed to obtain a response: %v", vmName, err)
// 			return nil, err
// 		}
// 	}

// 	returnStatus := &cloudyvm.VirtualMachineStatus{}
// 	returnStatus.Name = *resp.VirtualMachine.Name
// 	returnStatus.LongID = *resp.VirtualMachine.ID
// 	returnStatus.ID = *resp.VirtualMachine.Properties.VMID
// 	returnStatus.Size = string(*resp.VirtualMachine.Properties.HardwareProfile.VMSize)
// 	returnStatus.ProvisioningState = *resp.VirtualMachine.Properties.ProvisioningState

// 	if resp.VirtualMachine.Tags != nil {
// 		returnStatus.Tags = make(map[string]*string)

// 		for key, value := range resp.VirtualMachine.Tags {
// 			returnStatus.Tags[key] = value

// 			if strings.Compare(key, "User Principal Name") == 0 {
// 				returnStatus.User = *value
// 			}

// 		}
// 	}

// 	for _, status := range resp.VirtualMachine.Properties.InstanceView.Statuses {
// 		if strings.Contains(*status.Code, "PowerState") {
// 			returnStatus.PowerState = strings.Split(*status.Code, "/")[1]
// 		} else if strings.Contains(*status.Code, "ProvisioningState") {
// 			if status.Time != nil {
// 				returnStatus.ProvisioningTime = *status.Time
// 			}
// 		}
// 	}

// 	if resp.VirtualMachine.Properties != nil &&
// 		resp.VirtualMachine.Properties.StorageProfile != nil &&
// 		resp.VirtualMachine.Properties.StorageProfile.OSDisk != nil &&
// 		resp.VirtualMachine.Properties.StorageProfile.OSDisk.OSType != nil {
// 		ostype := *resp.VirtualMachine.Properties.StorageProfile.OSDisk.OSType
// 		returnStatus.OperatingSystem = string(ostype)
// 	}

// 	return returnStatus, err
// }

func VMGetPowerState(vm *armcompute.VirtualMachine) string {
	if vm == nil || vm.Properties == nil || vm.Properties.InstanceView == nil {
		return "NO POWERSTATE"
	}

	for _, status := range vm.Properties.InstanceView.Statuses {
		if strings.Contains(*status.Code, "PowerState") {
			return strings.Split(*status.Code, "/")[1]
		}
	}
	return ""
}

func VMAddTag(ctx context.Context) {

}

// func VmState(ctx context.Context, vmClient *armcompute.VirtualMachinesClient, vmAction cloudyvm.VirtualMachineAction, vmName string, resourceGroup string, wait bool) (*cloudyvm.VirtualMachineStatus, error) {
// 	if ctx == nil {
// 		ctx = cloudy.StartContext()
// 	}

// 	var err error

// 	if vmAction == cloudyvm.VirtualMachineStart {
// 		err = VmStart(ctx, vmClient, vmName, resourceGroup, wait)
// 	} else if vmAction == cloudyvm.VirtualMachineStop {
// 		err = VmStop(ctx, vmClient, vmName, resourceGroup, wait)
// 	} else if vmAction == cloudyvm.VirtualMachineTerminate {
// 		err = VmTerminate(ctx, vmClient, vmName, resourceGroup, wait)
// 	} else {
// 		err = fmt.Errorf("invalid state requested: %s", vmAction)
// 		return nil, err
// 	}

// 	if err != nil {
// 		return nil, err
// 	}

// 	vmStatus, err := VmStatus(ctx, vmClient, vmName, resourceGroup)

// 	return vmStatus, err
// }

// func VmStart(ctx context.Context, vmClient *armcompute.VirtualMachinesClient, vmName string, resourceGroup string, wait bool) error {
// 	if ctx == nil {
// 		ctx = cloudy.StartContext()
// 	}

// 	poller, err := vmClient.BeginStart(ctx, resourceGroup, vmName, &armcompute.VirtualMachinesClientBeginStartOptions{})

// 	if err != nil {
// 		_ = cloudy.Error(ctx, "[%s] BeginStart Failed to obtain a response: %v", vmName, err)
// 		return err
// 	}

// 	if wait {
// 		_, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{
// 			Frequency: 30 * time.Second,
// 		})
// 		if err != nil {
// 			_ = cloudy.Error(ctx, "[%s] Failed to start the vm: %v", vmName, err)
// 			return err
// 		}

// 		_ = cloudy.Error(ctx, "[%s] Started", vmName)
// 	}

// 	validatePowerState(ctx, vmClient, vmName, resourceGroup)

// 	return nil
// }

// func VmStop(ctx context.Context, vmClient *armcompute.VirtualMachinesClient, vmName string, resourceGroup string, wait bool) error {
// 	if ctx == nil {
// 		ctx = cloudy.StartContext()
// 	}

// 	poller, err := vmClient.BeginPowerOff(ctx, resourceGroup, vmName, &armcompute.VirtualMachinesClientBeginPowerOffOptions{})

// 	if err != nil {
// 		_ = cloudy.Error(ctx, "[%s] BeginPowerOff Failed to obtain a response: %v", vmName, err)
// 		return err
// 	}

// 	if wait {
// 		_, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{
// 			Frequency: 30 * time.Second,
// 		})
// 		if err != nil {
// 			_ = cloudy.Error(ctx, "[%s] Failed to stop the vm: %v", vmName, err)
// 			return err
// 		}

// 		cloudy.Info(ctx, "[%s] Stopped", vmName)
// 	}

// 	validatePowerState(ctx, vmClient, vmName, resourceGroup)

// 	return nil
// }

// func VmCreate(ctx context.Context, vmClient *armcompute.VirtualMachinesClient, vm *cloudyvm.VirtualMachineConfiguration) (*cloudyvm.VirtualMachineConfiguration, error) {
// 	return nil, nil
// }

// func VmTerminate(ctx context.Context, vmClient *armcompute.VirtualMachinesClient, vmName string, resourceGroup string, wait bool) error {
// 	cloudy.Info(ctx, "[%s] Starting VmTerminate (cloudy-azure>vm-connection)", vmName)

// 	poller, err := vmClient.BeginDeallocate(ctx, resourceGroup, vmName, &armcompute.VirtualMachinesClientBeginDeallocateOptions{})
// 	if err != nil {
// 		if is404(err) {
// 			cloudy.Info(ctx, "[%s] VmTerminate VM not found", vmName)
// 			return nil
// 		}

// 		_ = cloudy.Error(ctx, "[%s] VmTerminate Failed to obtain a response: %v", vmName, err)
// 		return err
// 	}

// 	if wait {
// 		_, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{
// 			Frequency: 30 * time.Second,
// 		})
// 		if err != nil {
// 			_ = cloudy.Error(ctx, "[%s] Failed to terminate the vm: %v", vmName, err)
// 			return err
// 		}

// 		cloudy.Info(ctx, "[%s] terminated ", vmName)
// 	}

// 	validatePowerState(ctx, vmClient, vmName, resourceGroup)

// 	return nil
// }

// func validatePowerState(ctx context.Context, vmClient *armcompute.VirtualMachinesClient, vmName string, resourceGroup string) {

// 	// max wait 30 seconds, no infinite loops allowed
// 	for i := 0; i < 30; i++ {
// 		vmStatus, err := VmStatus(ctx, vmClient, vmName, resourceGroup)

// 		if vmStatus.PowerState != "" {
// 			return
// 		}

// 		if err != nil {
// 			cloudy.Info(ctx, "Unable to validate power state, continuing (%s)", vmName)
// 			return
// 		}

// 		cloudy.Info(ctx, "Waiting for valid power state (%s)", vmName)

// 		time.Sleep(1 * time.Second)
// 	}

// }
