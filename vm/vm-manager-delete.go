package vm

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy/logging"
	"github.com/pkg/errors"
)

// Delete a VM, first by deallocating it, then deleting it and its associated resources
func (vmm *AzureVirtualMachineManager) DeleteVirtualMachine(ctx context.Context, vmId string) error {
	log := logging.GetLogger(ctx)

	if vmm.vmClient == nil {
		return errors.New("vmm.Delete: VM Client not initialized")
	}

	if vmm.Credentials == nil || vmm.Credentials.ResourceGroup == "" {
		return errors.New("vmm.Delete: credentials == nil or Resource Group not set")
	}

	if vmId == "" {
		return errors.New("vmm.Delete: VM ID not set")
	}

	log.InfoContext(ctx, "DeleteVM Starting Deallocate")
	err := vmm.deallocateVirtualMachine(ctx, vmId)
	if err != nil {
		return err
	}

	log.InfoContext(ctx, "DeleteVM Starting BeginDelete")
	poller, err := vmm.vmClient.BeginDelete(ctx, vmm.Credentials.ResourceGroup, vmId, &armcompute.VirtualMachinesClientBeginDeleteOptions{})
	if err != nil {
		if cloudyazure.Is404(err) {
			log.InfoContext(ctx, "BeginDelete vm not found")
		} else {
			return errors.Wrap(err, "VM Delete: BeginDelete")
		}
	} else {
		_, err = cloudyazure.PollWrapper(ctx, poller, "VM Delete")
		if err != nil {
			return errors.Wrap(err, "VM Delete: BeginDelete: polling")
		}
	}

	status, err := vmm.GetVirtualMachineById(ctx, vmId, true)
	if err != nil {
		return errors.Wrap(err, "VM Delete: Validate deleted")
	}
	if status != nil {
		return errors.New("VM Delete: VM not deleted")
	}

	log.InfoContext(ctx, "GetVmOsDisk")
	osDisk, err := vmm.GetOsDisk(ctx, vmId)
	if err != nil {
		return errors.Wrap(err, "VM Delete")
	}

	if osDisk != nil {
		log.InfoContext(ctx, "Starting DeleteDisk")
		err = vmm.DeleteDisk(ctx, osDisk.Name)
		if err != nil {
			return errors.Wrap(err, "VM Delete")
		}
	} else {
		log.InfoContext(ctx, "No OS Disk found")
	}

	log.InfoContext(ctx, "Starting GetNics")
	nics, err := vmm.GetNics(ctx, vmId)
	if err != nil {
		return errors.Wrap(err, "VM Delete")
	}

	if len(nics) > 0 {
		log.InfoContext(ctx, "Starting DeleteNics")
		err = vmm.DeleteNics(ctx, nics)
		if err != nil {
			return errors.Wrap(err, "VM Delete")
		}
	} else {
		log.InfoContext(ctx, "No Nics found")
	}

	return nil
}
