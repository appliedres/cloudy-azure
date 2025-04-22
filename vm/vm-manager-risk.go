package vm

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	"github.com/pkg/errors"
)

func (vmm *AzureVirtualMachineManager) GetAllDisks(ctx context.Context) ([]*models.VirtualMachineDisk, error) {
	disks := []*models.VirtualMachineDisk{}

	pager := vmm.diskClient.NewListPager(&armcompute.DisksClientListOptions{})

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "GetAllDisks")
		}

		for _, v := range resp.Value {
			disk := models.VirtualMachineDisk{
				ID:     *v.ID,
				Name:   *v.Name,
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

func (vmm *AzureVirtualMachineManager) DeleteDisk(ctx context.Context, diskName string) error {
	log := logging.GetLogger(ctx)

	pollerResponse, err := vmm.diskClient.BeginDelete(ctx, vmm.Credentials.ResourceGroup, diskName, nil)
	if err != nil {
		if cloudyazure.Is404(err) {
			log.InfoContext(ctx, fmt.Sprintf("Cannot delete, disk not found: %s", diskName))
			return nil
		}
		return fmt.Errorf("failed to begindelete disk %s (%v)", diskName, err)
	}

	_, err = pollerResponse.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to polluntildone disk %s (%v)", diskName, err)
	}

	return nil
}

func (vmm *AzureVirtualMachineManager) GetOsDisk(ctx context.Context, vmId string) (*models.VirtualMachineDisk, error) {
	disks, err := vmm.GetAllDisks(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GetOsDisk")
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
