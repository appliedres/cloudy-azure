package cloudyazure

import (
	"context"
	"fmt"

	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	"github.com/pkg/errors"
)

func (vmm *AzureVirtualMachineManager) Update(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachine, error) {
	// vm is the updated VM
	// VMID = vm.id, no need to pass in this separately since we have it in the updated vm object


	// do non-power cycle changes
	// TODO: vm.name
	// TODO: vm.tags
	// TODO: vm.description

	// power off changes
	// TODO: vm.nics
	// TODO: vm.osDisk
	// TODO: vm.disks
	// TODO: vm.apps
	// TODO: vm.template.size
	
	log := logging.GetLogger(ctx)

	if vm.ID == "" {
		return nil, fmt.Errorf("Cannot update a VM without an ID")
	}

	log.InfoContext(ctx, "VM Update starting")

	// Get current VM details
	currentVM, err := vmm.GetById(ctx, vm.ID, true)  // might need to GET with true and false includeState
	if err != nil {
		return nil, errors.Wrap(err, "VM Update, Get current VM")
	}

	requiresShutdown := false

	// Check if VM Family needs to be updated
	if vm.Template.Size != nil && *vm.Template.Size.Family != *currentVM.Template.Size.Family {
		log.InfoContext(ctx, "VM Update requires a size change")
		requiresShutdown = true
	}

	// Shutdown VM if required
	if requiresShutdown {
		log.InfoContext(ctx, "VM Update shutting down the VM")
		vmm.Deallocate(ctx, vm.ID)
	}

	// Update the VM
	log.InfoContext(ctx, "VM Update applying changes")
	virtualMachineParameters := FromCloudyVirtualMachine(ctx, currentVM)
	poller, err := vmm.vmClient.BeginCreateOrUpdate(ctx,
		vmm.credentials.ResourceGroup, vm.ID, virtualMachineParameters, nil)
	if err != nil {
		return nil, errors.Wrap(err, "VM Update, applying changes")
	}

	response, err := pollWrapper(ctx, poller, "VM Update")
	if err != nil {
		return nil, errors.Wrap(err, "VM Update, poller")
	}

	// Start the VM if it was shut down
	if requiresShutdown {
		log.InfoContext(ctx, "VM Update starting the VM")
		vmm.Start(ctx, vm.ID)
	}

	// Update the local model with the response
	err = UpdateCloudyVirtualMachine(vm, response.VirtualMachine)
	if err != nil {
		return nil, errors.Wrap(err, "VM Update, update model")
	}

	log.InfoContext(ctx, "VM Update completed successfully")
	return vm, nil
}


	// other VM fields for reference: skip these for now
	// vm.template.fromTemplateId
	// vm.template.name
	// vm.template.description
	// vm.template.iconPath
	// vm.template.bannerPath
	// vm.template.notes
	// vm.template.localAdministratorId
	// vm.template.ownerUserId
	// vm.template.ownerGroupId
	// vm.template.allowedUserIds
	// vm.template.allowedGroupIds
	// vm.template.virtualMachinePoolIds
	// vm.template.vdiTypes
	// vm.template.operatingSystem
	// vm.template.osBaseImageId
	// vm.template.minCpu
	// vm.template.maxCpu
	// vm.template.cpuVendor
	// vm.template.cpuGeneration
	// vm.template.minRam
	// vm.template.maxRam
	// vm.template.minNic
	// vm.template.maxNic
	// vm.template.acceleratedNetworking
	// vm.template.minGpu
	// vm.template.maxGpu
	// vm.template.gpuVendor
	// vm.template.disks
	// vm.template.apps
	// vm.template.tags
	// vm.template.featured
	// vm.template.timeout

	// won't / can't change
	// vm.id  // vm id is permanent
	// vm.template  // vm template is tied to VM, should never change
	// vm.template.id  // template id is permanent
	// vm.creatorId  // determined at creation, doesn't change
	// vm.cloudState  // determined from azure
	// vm.status  // should not be manually changed, only handled via VM process
	// vm.connect  // avd connect info shouldn't change outside of the AVD register process

	// Maybe, but not now
	// vm.userId  // would require significant work to reassign this to another user, mostly involving AVD
	// vm.location  // would have to use different credentials, possible but complicated

	// handled elsewhere
	// vm.activity
	// vm.logs
	// vm.estimatedCostPerHour  // handled by cost tracking
	// vm.estimatedCostAccumulated  // handled by cost tracking