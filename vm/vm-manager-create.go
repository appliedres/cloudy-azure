package vm

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	"github.com/pkg/errors"
)

const (
	vmNameTagKey    = "Name"
	vmIDTagKey      = "VMID"
	vmCreatorTagKey = "CreatorID"
	vmUserTagKey    = "UserID"
	vmTeamTagKey    = "TeamID"
	createdByTagKey = "CreatedBy"
)

func generateAzureTagsForVM(vm *models.VirtualMachine) map[string]*string {
	tags := make(map[string]*string)

	if vm.Template != nil {
		for k, v := range vm.Template.Tags {
			tags[k] = v
		}
	}

	if vm.Tags != nil {
		for k, v := range vm.Tags {
			tags[k] = v
		}
	}

	// Add vm fields as tags for azure resources
	if vm.Name != "" {
		tags[vmNameTagKey] = to.Ptr(vm.Name)
	}
	if vm.ID != "" {
		tags[vmIDTagKey] = to.Ptr(vm.ID)
	}
	if vm.CreatorID != "" {
		tags[vmCreatorTagKey] = to.Ptr(vm.CreatorID)
	}
	if vm.UserID != "" {
		tags[vmUserTagKey] = to.Ptr(vm.UserID)
	}
	if vm.TeamID != "" {
		tags[vmTeamTagKey] = to.Ptr(vm.TeamID)
	}

	// other metadata
	tags[createdByTagKey] = to.Ptr("cloudy-azure")

	return tags
}

func (vmm *AzureVirtualMachineManager) CreateVirtualMachine(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	log := logging.GetLogger(ctx)

	if vm.ID == "" {
		return nil, fmt.Errorf("cannot create a VM without an ID")
	}

	log.InfoContext(ctx, "VM Create starting")

	if vm.Location == nil {
		vm.Location = &models.VirtualMachineLocation{
			Cloud:  cloudyazure.CloudAzureUSGovernment,
			Region: vmm.Credentials.Region,
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
		vmm.Credentials.ResourceGroup, vm.ID, *virtualMachineParameters, nil)
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
