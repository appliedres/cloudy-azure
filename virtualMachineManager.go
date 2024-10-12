package cloudyazure

import (
	"context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/models"
	cloudyvm "github.com/appliedres/cloudy/vm"
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

	dataClient  *armcompute.ResourceSKUsClient
	usageClient *armcompute.UsageClient

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

	dataClient, err := armcompute.NewResourceSKUsClient(vmm.credentials.SubscriptionID, credential, options)
	if err != nil {
		return err
	}
	vmm.dataClient = dataClient

	usageClient, err := armcompute.NewUsageClient(vmm.credentials.SubscriptionID, credential, options)
	if err != nil {
		return err
	}

	vmm.usageClient = usageClient

	return nil
}

func (vmm *AzureVirtualMachineManager) Update(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	return nil, nil
}

func (vmm *AzureVirtualMachineManager) GetById(ctx context.Context, id string) (*models.VirtualMachine, error) {
	return nil, nil
}

func (vmm *AzureVirtualMachineManager) GetAll(ctx context.Context, filter string, attrs []string) (*[]models.VirtualMachine, error) {

	vmList := []models.VirtualMachine{}

	statusOnly := "false"

	pager := vmm.vmClient.NewListAllPager(&armcompute.VirtualMachinesClientListAllOptions{
		StatusOnly: &statusOnly,
	})

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return &vmList, err
		}

		for _, vm := range resp.Value {
			cloudyVm := ToCloudyVirtualMachine(vm)
			vmList = append(vmList, *cloudyVm)
		}

	}

	return &vmList, nil
}

func (vmm *AzureVirtualMachineManager) Start(ctx context.Context, id string) error {
	return nil
}

func (vmm *AzureVirtualMachineManager) Stop(ctx context.Context, id string) error {
	return nil
}

func (vmm *AzureVirtualMachineManager) GetData(ctx context.Context) (map[string]models.VirtualMachineSize, error) {

	dataList := map[string]models.VirtualMachineSize{}

	pager := vmm.dataClient.NewListPager(&armcompute.ResourceSKUsClientListOptions{})
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return dataList, cloudy.Error(ctx, "could not get NextPage, %v", err)
		}

		for _, v := range resp.Value {
			if *v.ResourceType == "virtualMachines" {

				vmData := ToCloudyVirtualMachineSize(ctx, v)

				dataList[vmData.Name] = *vmData
			}
		}
	}

	return dataList, nil
}

func (vmm *AzureVirtualMachineManager) GetUsage(ctx context.Context) (map[string]models.VirtualMachineFamily, error) {

	usageList := map[string]models.VirtualMachineFamily{}

	pager := vmm.usageClient.NewListPager(vmm.credentials.Location, &armcompute.UsageClientListOptions{})

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return usageList, cloudy.Error(ctx, "could not get NextPage, %v", err)
		}

		for _, v := range resp.Value {
			if strings.HasSuffix(*v.Name.Value, "Family") {
				family := models.VirtualMachineFamily{
					Name:  *v.Name.Value,
					Usage: int64(*v.CurrentValue),
					Quota: *v.Limit,
				}

				usageList[family.Name] = family
			}

		}

	}

	return usageList, nil
}

func (vmm *AzureVirtualMachineManager) GetDataWithUsage(ctx context.Context) (map[string]models.VirtualMachineSize, error) {

	sizes, err := vmm.GetData(ctx)
	if err != nil {
		return nil, err
	} else {
		usage, err := vmm.GetUsage(ctx)
		if err != nil {
			return nil, err
		}

		for sizeName, size := range sizes {
			u, ok := usage[size.Family.ID]

			if !ok {
				cloudy.Warn(ctx, "size %s family %s missing in usage", size.ID, size.Family.ID)
			} else {
				size.Family.Usage = u.Usage
				size.Family.Quota = u.Quota

				sizes[sizeName] = size
			}
		}

	}

	return sizes, nil
}

func UpdateCloudyVirtualMachine(vm *models.VirtualMachine, responseVirtualMachine armcompute.VirtualMachine) error {

	return nil
}
