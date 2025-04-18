package storage

import (
	"context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/appliedres/cloudy"
	cloudyazure "github.com/appliedres/cloudy-azure"
)

func GetStorageAccountType(ctx context.Context, env *cloudy.Environment, name string) (string, error) {

	azureCred := cloudyazure.GetAzureCredentialsFromEnv(env)
	subscriptionId := env.Force("AZ_SUBSCRIPTION_ID")
	resourceGroup := env.Force("AZ_RESOURCE_GROUP")

	cred, err := cloudyazure.GetAzureClientSecretCredential(azureCred)
	if err != nil {
		return "", err
	}

	var clientOptions policy.ClientOptions
	clientOptions.Cloud = cloud.AzureGovernment

	clientFactory, err := armstorage.NewAccountsClient(subscriptionId, cred, &clientOptions)
	if err != nil {
		return "", err
	}

	pager := clientFactory.NewListByResourceGroupPager(resourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", err
		}

		for _, v := range page.Value {
			if strings.EqualFold(*v.Name, name) {
				cloudy.Info(ctx, "Storage account %s found: type: %s", *v.Name, *v.Kind)
				return string(*v.Kind), nil
			}
		}
	}

	return "", cloudy.Error(ctx, "Storage account %s not found", name)
}
