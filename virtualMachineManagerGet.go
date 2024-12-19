package cloudyazure

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	"github.com/pkg/errors"
)

func (vmm *AzureVirtualMachineManager) GetById(ctx context.Context, id string, statusOnly bool) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)

	expandGet := to.Ptr(armcompute.InstanceViewTypesInstanceView)  // Always get the status

	resp, err := vmm.vmClient.Get(ctx, vmm.credentials.ResourceGroup, id, &armcompute.VirtualMachinesClientGetOptions{
		Expand: expandGet,
	})
	if err != nil {
		if is404(err) {
			log.InfoContext(ctx, fmt.Sprintf("Azure vmm.GetById VM not found: [%s]", id))
			return nil, nil
		}

		return nil, errors.Wrap(err, "Azure vmm.GetById Error")
	}

	vm := ToCloudyVirtualMachine(ctx, &resp.VirtualMachine)
	log.DebugContext(ctx, fmt.Sprintf("Azure vmm.GetById: vmid:[%s] state:[%s] status:[%s]", id, vm.State, vm.Status))

	return vm, nil
}

func (vmm *AzureVirtualMachineManager) GetAll(ctx context.Context, filter string, attrs []string, statusOnly bool) (*[]models.VirtualMachine, error) {

	vmList := []models.VirtualMachine{}

	statusOnlyString := strconv.FormatBool(statusOnly)

	pager := vmm.vmClient.NewListAllPager(&armcompute.VirtualMachinesClientListAllOptions{
		StatusOnly: &statusOnlyString,
	})

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return &vmList, err
		}

		for _, vm := range resp.Value {
			cloudyVm := ToCloudyVirtualMachine(ctx, vm)
			vmList = append(vmList, *cloudyVm)
		}

	}

	return &vmList, nil
}