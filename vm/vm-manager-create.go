package vm

import (
	"context"
	"fmt"

	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	"github.com/pkg/errors"
)

func (vmm *AzureVirtualMachineManager) Create(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)

	if vm.ID == "" {
		return nil, fmt.Errorf("cannot create a VM without an ID")
	}

	log.InfoContext(ctx, "VM Create starting")

	if vm.Location == nil {
		vm.Location = &models.VirtualMachineLocation{
			Cloud:  cloudyazure.CloudAzureUSGovernment,
			Region: vmm.credentials.Region,
		}
	}

	log.InfoContext(ctx, "VM Create creating nics")

	nics, err := vmm.GetNics(ctx, vm.ID)
	if err != nil {
		return nil, errors.Wrap(err, "VM Create, Get NICs")
	} else if len(nics) != 0 {
		vm.Nics = nics
	} else {
		newNic, err := vmm.CreateNic(ctx, vm)
		if err != nil {
			return nil, errors.Wrap(err, "VM Create, Create NIC")
		}
		vm.Nics = append(vm.Nics, newNic)
	}

	log.InfoContext(ctx, "VM Create converting from cloudy to azure")

	virtualMachineParameters, err := FromCloudyVirtualMachine(ctx, vm)
	if err != nil {
		return nil, errors.Wrap(err, "VM Create, FromCloudyVirtualMachine failed")
	}

	log.InfoContext(ctx, "VM Create BeginCreateOrUpdate starting")

	poller, err := vmm.vmClient.BeginCreateOrUpdate(ctx,
		vmm.credentials.ResourceGroup, vm.ID, *virtualMachineParameters, nil)
	if err != nil {
		return nil, errors.Wrap(err, "VM Create")
	}

	response, err := cloudyazure.PollWrapper(ctx, poller, "VM Create")
	if err != nil {
		return nil, errors.Wrap(err, "VM Create")
	}

	err = UpdateCloudyVirtualMachine(vm, response.VirtualMachine)
	if err != nil {
		return nil, errors.Wrap(err, "VM Create")
	}

	return vm, nil
}
