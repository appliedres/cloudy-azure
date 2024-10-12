package cloudyazure

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/logging"
)

func (vmm *AzureVirtualMachineManager) Deallocate(ctx context.Context, vmId string) error {

	poller, err := vmm.vmClient.BeginDeallocate(ctx, vmm.credentials.ResourceGroup, vmId, &armcompute.VirtualMachinesClientBeginDeallocateOptions{})
	if err != nil {
		if is404(err) {
			cloudy.Info(ctx, "[%s] VmTerminate VM not found", vmId)
			return nil
		}

		_ = cloudy.Error(ctx, "[%s] VmTerminate Failed to obtain a response: %v", vmId, err)
		return err
	}

	_, err = poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{
		Frequency: 5 * time.Second,
	})
	if err != nil {
		_ = cloudy.Error(ctx, "[%s] Failed to terminate the vm: %v", vmId, err)
		return err
	}

	cloudy.Info(ctx, "[%s] terminated ", vmId)

	return nil
}

func (vmm *AzureVirtualMachineManager) Delete(ctx context.Context, vmId string) error {
	log := logging.GetLogger(ctx)

	err := vmm.Deallocate(ctx, vmId)
	if err != nil {
		return err
	}

	log.InfoContext(ctx, fmt.Sprintf("[%s] Starting GetVmOsDisk", vmId))
	osDisk, err := vmm.GetOsDisk(ctx, vmId)
	if err != nil {
		log.ErrorContext(ctx, fmt.Sprintf("[%s] failed to find vm os disk: %v", vmId, err))
		return err
	}

	if osDisk != nil {
		cloudy.Info(ctx, "[%s] Starting DeleteDisk", vmId)
		err = vmm.DeleteDisk(ctx, osDisk.ID)
		if err != nil {
			log.ErrorContext(ctx, fmt.Sprintf("[%s] %v", vmId, err), logging.WithError(err))
			return err
		}
	} else {
		cloudy.Info(ctx, "[%s] No OS Disk found", vmId)
	}

	cloudy.Info(ctx, "[%s] Starting GetNics", vmId)
	nics, err := vmm.GetNics(ctx, vmId)
	if err != nil {
		log.ErrorContext(ctx, fmt.Sprintf("[%s] failed to find vm nic: %v", vmId, err))
		return err
	}

	if len(nics) > 0 {
		cloudy.Info(ctx, "[%s] Starting DeleteNIC", vmId)
		err = vmm.DeleteNics(ctx, nics)
		if err != nil {
			log.ErrorContext(ctx, fmt.Sprintf("[%s] %v", vmId, err))
			return err
		}
	} else {
		cloudy.Info(ctx, "[%s] No Nics found", vmId)
	}

	return nil
}
