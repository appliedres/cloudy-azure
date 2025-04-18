package vm

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	"github.com/pkg/errors"
)

// Queries Azure for the details of a single VM.
//
//	If includeState is true, this will also retrieve the state of the VM (running, stopped, etc.)
//	If includeState is false, vm.State will be an empty string
func (vmm *AzureVirtualMachineManager) GetVirtualMachine(ctx context.Context, vmName string, includeState bool) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)

	var expandGet *armcompute.InstanceViewTypes
	if includeState {
		expandGet = to.Ptr(armcompute.InstanceViewTypesInstanceView)
	}
	resp, err := vmm.vmClient.Get(ctx, vmm.Credentials.ResourceGroup, vmName, &armcompute.VirtualMachinesClientGetOptions{
		Expand: expandGet,
	})
	if err != nil {
		if cloudyazure.Is404(err) {
			log.DebugContext(ctx, fmt.Sprintf("Azure vmm.GetById VM not found: [%s]", vmName))
			return nil, nil
		}

		return nil, errors.Wrap(err, "Azure vmm.GetById Error")
	}

	vm := ToCloudyVirtualMachine(ctx, &resp.VirtualMachine)

	stateString := ""
	if vm.CloudState != nil {
		stateString = string(*vm.CloudState)
	}
	log.DebugContext(ctx, fmt.Sprintf("Azure vmm.GetById: vmid:[%s] state:[%s]", vmName, stateString))

	return vm, nil
}

// Queries Azure for the details of all VMs.
//
//	If includeState is true, this will also retrieve the state of the VMs (running, stopped, etc.)
//	If includeState is false, vm.State will be an empty string
func (vmm *AzureVirtualMachineManager) GetAllVirtualMachines(ctx context.Context, filter string, attrs []string, includeState bool) (*[]models.VirtualMachine, error) {

	vmList := []models.VirtualMachine{}

	statusOnlyString := strconv.FormatBool(includeState)

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
