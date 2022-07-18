package cloudyazure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/appliedres/cloudy"
)

type AzureVMSizeCache struct {
	sizes map[string]*AzVmSize
}

func (azs *AzureVMSizeCache) Load(ctx context.Context, vmc *AzureVMController) error {
	client, err := armcompute.NewResourceSKUsClient(vmc.Config.SubscriptionID, vmc.cred, &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	})
	if err != nil {
		return cloudy.Error(ctx, "could not create NewResourceSKUsClient, %v", err)
	}

	azs.sizes = make(map[string]*AzVmSize)
	pager := client.NewListPager(&armcompute.ResourceSKUsClientListOptions{})
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return cloudy.Error(ctx, "could not get NextPage, %v", err)
		}

		for _, r := range resp.Value {
			if *r.ResourceType == "virtualMachines" {
				size := SizeFromResource(ctx, r)
				azs.sizes[size.Name] = size

				if strings.HasPrefix(size.Size, "N") {
					fmt.Println("STOP")
				}
			}
		}
	}

	return nil
}

func (azs *AzureVMSizeCache) GetEquivilent(ctx context.Context, vCPUmin int, vCPUmax int, minMemoryGB float64, vGPUmin float64, vGPUmax float64, gpuVendor string) []*AzVmSize {
	var rtn []*AzVmSize

	for _, size := range azs.sizes {
		if size.VCPU < vCPUmin {
			continue
		}
		if size.VCPU > vCPUmin {
			continue
		}
		if size.GPU < vGPUmin {
			continue
		}
		if size.GPU > vGPUmax {
			continue
		}
		if size.MemoryGB < minMemoryGB {
			continue
		}
		if size.GpuVendor != gpuVendor {
			continue
		}
		rtn = append(rtn, size)
	}

	return rtn
}
