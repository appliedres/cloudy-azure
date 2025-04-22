package vm

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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

// Queries Azure for the details of all User VMs.
//
//	If includeState is true, this will also retrieve the state of the VMs (running, stopped, etc.)
//	If includeState is false, vm.State will be an empty string
func (vmm *AzureVirtualMachineManager) GetAllUserVirtualMachines(ctx context.Context, attrs []string, includeState bool) (*[]models.VirtualMachine, error) {
	return vmm.getAllVirtualMachinesWithPrefix(ctx, "uvm-", attrs, includeState)
}

// Queries Azure for the details of all Session Host VMs.
//
//	If includeState is true, this will also retrieve the state of the VMs (running, stopped, etc.)
//	If includeState is false, vm.State will be an empty string
func (vmm *AzureVirtualMachineManager) GetAllSessionHostVirtualMachines(ctx context.Context, attrs []string, includeState bool) (*[]models.VirtualMachine, error) {
	return vmm.getAllVirtualMachinesWithPrefix(ctx, "shvm-", attrs, includeState)
}

// Warning: this could return critical infrastructure VMs if filter is not specified
func (vmm *AzureVirtualMachineManager) getAllVirtualMachinesWithPrefix(ctx context.Context, filterPrefix string, attrs []string, includeState bool) (*[]models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)

	if filterPrefix == "" {
		log.WarnContext(ctx, "Querying VMs without a filter. This could return critical infrastructure VMs")
	}

    vmList := []models.VirtualMachine{}

    statusOnlyString := strconv.FormatBool(includeState)

    options := &armcompute.VirtualMachinesClientListAllOptions{
        StatusOnly: &statusOnlyString,
    }
    pager := vmm.vmClient.NewListAllPager(options)

    for pager.More() {
        resp, err := pager.NextPage(ctx)
        if err != nil {
            return &vmList, err
        }

        for _, armVM := range resp.Value {
            // If a prefix filter was provided, skip any VM that doesn't match
            if filterPrefix != "" {
                if armVM.Name == nil || !strings.HasPrefix(*armVM.Name, filterPrefix) {
                    continue
                }
            }

            cloudyVM := ToCloudyVirtualMachine(ctx, armVM)
            vmList = append(vmList, *cloudyVM)
        }
    }

    return &vmList, nil
}
