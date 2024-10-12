package cloudyazure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
)

func (vmm *AzureVirtualMachineManager) Create(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)

	if vm.ID == "" {
		return nil, fmt.Errorf("New VM Id must be specified")

	}

	if vm.Location == nil {
		vm.Location = &models.VirtualMachineLocation{
			Cloud:  "azure",
			Region: vmm.credentials.Location,
		}
	}

	nics, err := vmm.GetNics(ctx, vm.ID)
	if err != nil {
		return nil, err
	} else if len(nics) != 0 {
		vm.Nics = nics
	} else {
		newNic, err := vmm.CreateNic(ctx, vm)
		if err != nil {
			return nil, err
		}
		vm.Nics = append(vm.Nics, newNic)
	}

	virtualMachineParameters := FromCloudyVirtualMachine(ctx, vm)

	poller, err := vmm.vmClient.BeginCreateOrUpdate(ctx,
		vmm.credentials.ResourceGroup, vm.Name, virtualMachineParameters, nil)
	if err != nil {

		respErr, ok := err.(*azcore.ResponseError)
		if ok {
			log.InfoContext(ctx, fmt.Sprintf("%v", respErr))
		} else {

		}

		return nil, err
	}

	response, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{})
	if err != nil {
		return nil, err
	}

	err = UpdateCloudyVirtualMachine(vm, response.VirtualMachine)
	if err != nil {
		return nil, err
	}

	return vm, nil
}
