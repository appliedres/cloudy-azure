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
	"github.com/appliedres/cloudy/datastore"
	cloudyvm "github.com/appliedres/cloudy/vm"
)

type AzureVMSizeCache struct {
	sizes map[string]*cloudyvm.VmSize
}

func (azs *AzureVMSizeCache) Merge(ctx context.Context, datatype datastore.Datatype[any]) {

}

func (azs *AzureVMSizeCache) Load(ctx context.Context, vmc *AzureVMController) error {
	cloudy.Info(ctx, "AzureVMSizeCache.Load")

	client, err := armcompute.NewResourceSKUsClient(vmc.Config.SubscriptionID, vmc.cred, &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	})
	if err != nil {
		return cloudy.Error(ctx, "could not create NewResourceSKUsClient, %v", err)
	}

	azs.sizes = make(map[string]*cloudyvm.VmSize)
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
