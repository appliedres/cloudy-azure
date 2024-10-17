package cloudyazure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	"github.com/pkg/errors"
)

func (vmm *AzureVirtualMachineManager) GetById(ctx context.Context, id string) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)

	resp, err := vmm.vmClient.Get(ctx, vmm.credentials.ResourceGroup, id, &armcompute.VirtualMachinesClientGetOptions{
		Expand: nil,
	})

	if err != nil {
		if is404(err) {
			log.InfoContext(ctx, fmt.Sprintf("GetById vm not found: %s", id))
			return nil, nil
		}

		return nil, errors.Wrap(err, "VM GetById")
	}

	vm := ToCloudyVirtualMachine(&resp.VirtualMachine)

	return vm, nil
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
