package cloudyazure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/appliedres/cloudy/models"
)

func (vmm *AzureVirtualMachineManager) GetAllDisks(ctx context.Context) ([]*models.VirtualMachineDisk, error) {
	disks := []*models.VirtualMachineDisk{}

	pager := vmm.diskClient.NewListPager(&armcompute.DisksClientListOptions{})

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, v := range resp.Value {
			disk := models.VirtualMachineDisk{
				ID:     *v.ID,
				OsDisk: false,
				Size:   int64(*v.Properties.DiskSizeGB),
			}

			if v.Properties.OSType != nil {
				disk.OsDisk = true
			}

			disks = append(disks, &disk)
		}
	}

	return disks, nil
}

func (vmm *AzureVirtualMachineManager) DeleteDisk(ctx context.Context, id string) error {
	pollerResponse, err := vmm.diskClient.BeginDelete(ctx, vmm.credentials.ResourceGroup, id, nil)
	if err != nil {
		return fmt.Errorf("failed to begindelete disk %s (%v)", id, err)
	}

	_, err = pollerResponse.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to polluntildone disk %s (%v)", id, err)
	}

	return nil
}

func (vmm *AzureVirtualMachineManager) GetOsDisk(ctx context.Context, vmId string) (*models.VirtualMachineDisk, error) {
	disks, err := vmm.GetAllDisks(ctx)
	if err != nil {
		return nil, err
	}

	for _, disk := range disks {
		// Match by type and name
		if disk.OsDisk && strings.Contains(disk.ID, vmId) {
			return disk, nil
		}
	}

	return nil, nil
}

func (vmm *AzureVirtualMachineManager) GetAllVmDisks(ctx context.Context, vmId string) ([]*models.VirtualMachineDisk, error) {
	disks := []*models.VirtualMachineDisk{}

	allDisks, err := vmm.GetAllDisks(ctx)
	if err != nil {
		return nil, err
	}

	for _, disk := range allDisks {
		// Match by name
		if strings.Contains(disk.ID, vmId) {
			disks = append(disks, disk)
		}
	}

	return disks, nil
}
