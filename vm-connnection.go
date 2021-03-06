package cloudyazure

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/appliedres/cloudy"
	cloudyvm "github.com/appliedres/cloudy/vm"
)

func NewVMClient(ctx context.Context, config *AzureVMControllerConfig) (*armcompute.VirtualMachinesClient, error) {
	cred, err := GetAzureCredentials(config.AzureCredentials)
	if err != nil {
		return nil, cloudy.Error(ctx, "Authentication failure: %+v", err)
	}

	client, err := armcompute.NewVirtualMachinesClient(config.SubscriptionID, cred,
		&arm.ClientOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloud.AzureGovernment,
			},
		})

	return client, err
}

func NewUsageClient(ctx context.Context, config *AzureVMControllerConfig) (*armcompute.UsageClient, error) {
	cred, err := GetAzureCredentials(config.AzureCredentials)
	if err != nil {
		return nil, cloudy.Error(ctx, "Authentication failure: %+v", err)
	}

	client, err := armcompute.NewUsageClient(config.SubscriptionID, cred,
		&arm.ClientOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloud.AzureGovernment,
			},
		})

	return client, err
}

func getVMClient(ctx context.Context) (*armcompute.VirtualMachinesClient, error) {
	if ctx == nil {
		ctx = cloudy.StartContext()
	}

	tenantID := os.Getenv("AZURE_COMPUTE_TENANT_ID")
	clientID := os.Getenv("AZURE_COMPUTE_CLIENT_ID")
	clientSecret := os.Getenv("AZURE_COMPUTE_CLIENT_SECRET")
	subscriptionID := os.Getenv("AZURE_COMPUTE_SUBSCRIPTION_ID")

	azConfig := AzureCredentials{
		TenantID:     tenantID,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}

	cred, err := GetAzureCredentials(azConfig)
	// cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret,
	// 	&azidentity.ClientSecretCredentialOptions{AuthorityHost: azidentity.AzureGovernment})

	if err != nil {
		cloudy.Error(ctx, "Authentication failure: %+v", err)
	}

	client, err := armcompute.NewVirtualMachinesClient(subscriptionID, cred,
		&arm.ClientOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloud.AzureGovernment,
			},
		})

	return client, err
}

func VmList(ctx context.Context, vmClient *armcompute.VirtualMachinesClient, rg string) ([]*cloudyvm.VirtualMachineStatus, error) {
	var err error
	if ctx == nil {
		ctx = cloudy.StartContext()
	}

	var returnList []*cloudyvm.VirtualMachineStatus

	// resourceGroup := os.Getenv("AZURE_COMPUTE_RESOURCE_GROUP")
	pager := vmClient.NewListAllPager(&armcompute.VirtualMachinesClientListAllOptions{})
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			cloudy.Error(ctx, "VM Client List Error %s", err)
			break
		}
		for _, vm := range resp.Value {
			// log.Printf("name: %s", *vm.Name)

			var vmStatus *cloudyvm.VirtualMachineStatus
			vmStatus, err = VmStatus(ctx, vmClient, *vm.Name, rg)

			if err == nil {
				returnList = append(returnList, vmStatus)
			}
		}
	}

	return returnList, err
}

func VmStatus(ctx context.Context, vmClient *armcompute.VirtualMachinesClient, vmName string, resourceGroup string) (*cloudyvm.VirtualMachineStatus, error) {
	if ctx == nil {
		ctx = cloudy.StartContext()
	}

	var err error

	if vmClient == nil {
		vmClient, err = getVMClient(ctx)

		if err != nil {
			return nil, err
		}
	}

	// resourceGroup := os.Getenv("AZURE_COMPUTE_RESOURCE_GROUP")

	// resp, err := vmClient.Get(context.Background(), resourceGroup, vmName, &armcompute.VirtualMachinesGetOptions{Expand: armcompute.InstanceViewTypesUserData.ToPtr()})
	// if err != nil {
	// 	log.Fatalf("failed to obtain a response: %v", err)
	// }

	instanceType := armcompute.InstanceViewTypesInstanceView
	resp, err := vmClient.Get(context.Background(), resourceGroup, vmName,
		&armcompute.VirtualMachinesClientGetOptions{
			Expand: &instanceType,
		})

	// if resp.RawResponse != nil && resp.RawResponse.StatusCode == 404 {
	// 	// This item was not found
	// 	return nil, nil
	// }
	if err != nil {
		cloudy.Error(ctx, "failed to obtain a response: %v", err)
		// Not returning error since "Not Found" is an error
		return nil, nil
	}

	returnStatus := &cloudyvm.VirtualMachineStatus{}
	returnStatus.Name = *resp.VirtualMachine.Name
	returnStatus.LongID = *resp.VirtualMachine.ID
	returnStatus.ID = *resp.VirtualMachine.Properties.VMID
	returnStatus.Size = string(*resp.VirtualMachine.Properties.HardwareProfile.VMSize)
	returnStatus.ProvisioningState = *resp.VirtualMachine.Properties.ProvisioningState

	if resp.VirtualMachine.Tags != nil {
		returnStatus.Tags = make(map[string]*string)

		for key, value := range resp.VirtualMachine.Tags {
			returnStatus.Tags[key] = value

			if strings.Compare(key, "User Principal Name") == 0 {
				returnStatus.User = *value
			}

		}
	}

	for _, status := range resp.VirtualMachine.Properties.InstanceView.Statuses {
		if strings.Contains(*status.Code, "PowerState") {
			returnStatus.PowerState = strings.Split(*status.Code, "/")[1]
		} else if strings.Contains(*status.Code, "ProvisioningState") {
			if status.Time != nil {
				returnStatus.ProvisioningTime = *status.Time
			}
		}
	}

	if resp.VirtualMachine.Properties != nil &&
		resp.VirtualMachine.Properties.StorageProfile != nil &&
		resp.VirtualMachine.Properties.StorageProfile.OSDisk != nil &&
		resp.VirtualMachine.Properties.StorageProfile.OSDisk.OSType != nil {
		ostype := *resp.VirtualMachine.Properties.StorageProfile.OSDisk.OSType
		returnStatus.OperatingSystem = string(ostype)
	}

	return returnStatus, err
}

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

func VmState(ctx context.Context, vmClient *armcompute.VirtualMachinesClient, vmAction cloudyvm.VirtualMachineAction, vmName string, resourceGroup string, wait bool) (*cloudyvm.VirtualMachineStatus, error) {
	if ctx == nil {
		ctx = cloudy.StartContext()
	}

	var vmStatus *cloudyvm.VirtualMachineStatus
	var err error = nil

	if vmAction == cloudyvm.VirtualMachineStart {
		err = VmStart(ctx, vmClient, vmName, resourceGroup, wait)
	} else if vmAction == cloudyvm.VirtualMachineStop {
		err = VmStop(ctx, vmClient, vmName, resourceGroup, wait)
	} else if vmAction == cloudyvm.VirtualMachineTerminate {
		err = VmTerminate(ctx, vmClient, vmName, resourceGroup, wait)
	} else {
		err = fmt.Errorf("invalid state requested: %s", vmAction)
		return vmStatus, err
	}

	if err != nil {
		return nil, err
	}

	vmStatus, err = VmStatus(ctx, vmClient, vmName, resourceGroup)

	return vmStatus, err
}

func VmStart(ctx context.Context, vmClient *armcompute.VirtualMachinesClient, vmName string, resourceGroup string, wait bool) error {
	if ctx == nil {
		ctx = cloudy.StartContext()
	}

	// resourceGroup := os.Getenv("AZURE_COMPUTE_RESOURCE_GROUP")

	poller, err := vmClient.BeginStart(ctx, resourceGroup, vmName, &armcompute.VirtualMachinesClientBeginStartOptions{})

	if err != nil {
		cloudy.Error(ctx, "failed to obtain a response: %v", err)
		return err
	}

	if wait {
		_, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{
			Frequency: 30 * time.Second,
		})
		if err != nil {
			cloudy.Error(ctx, "failed to start the vm: %v", err)
			return err
		}

		cloudy.Error(ctx, "start response")

		return err
	}
	return nil
}

func VmStop(ctx context.Context, vmClient *armcompute.VirtualMachinesClient, vmName string, resourceGroup string, wait bool) error {
	if ctx == nil {
		ctx = cloudy.StartContext()
	}

	// resourceGroup := os.Getenv("AZURE_COMPUTE_RESOURCE_GROUP")

	poller, err := vmClient.BeginPowerOff(ctx, resourceGroup, vmName, &armcompute.VirtualMachinesClientBeginPowerOffOptions{})

	if err != nil {
		cloudy.Error(ctx, "failed to obtain a response: %v", err)
		return err
	}

	if wait {
		_, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{
			Frequency: 30 * time.Second,
		})
		if err != nil {
			cloudy.Error(ctx, "failed to start the vm: %v", err)
			return err
		}

		cloudy.Info(ctx, "stop")

		return err
	}

	return nil
}

func VmCreate(ctx context.Context, vmClient *armcompute.VirtualMachinesClient, vm *cloudyvm.VirtualMachineConfiguration) (*cloudyvm.VirtualMachineConfiguration, error) {
	return nil, nil
}

func VmTerminate(ctx context.Context, vmClient *armcompute.VirtualMachinesClient, vmName string, resourceGroup string, wait bool) error {
	if ctx == nil {
		ctx = cloudy.StartContext()
	}
	// resourceGroup := os.Getenv("AZURE_COMPUTE_RESOURCE_GROUP")

	poller, err := vmClient.BeginDeallocate(ctx, resourceGroup, vmName, &armcompute.VirtualMachinesClientBeginDeallocateOptions{})

	if err != nil {
		cloudy.Error(ctx, "failed to obtain a response: %v", err)
		return err
	}

	if wait {
		_, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{
			Frequency: 30 * time.Second,
		})
		if err != nil {
			cloudy.Error(ctx, "failed to start the vm: %v", err)
			return err
		}

		cloudy.Info(ctx, "terminate ")

		return err
	}

	return nil
}
