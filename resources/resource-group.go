package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy/logging"
)

type ResourceGroupConfig struct {
	PollingTimeoutDuration string
}

type ResourceGroupManager struct {
	credentials *cloudyazure.AzureCredentials
	token       azcore.TokenCredential
	config      *ResourceGroupConfig
}

func NewResourceGroupManager(ctx context.Context, config *ResourceGroupConfig, creds *cloudyazure.AzureCredentials) (*ResourceGroupManager, error) {
	log := logging.GetLogger(ctx)

	resourceGroup := &ResourceGroupManager{
		credentials: creds,
	}

	resourceGroup.config = setConfig(config)

	cred, err := cloudyazure.NewAzureCredentials(creds)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create azure token", logging.WithError(err))
		return nil, err
	}

	resourceGroup.token = cred
	return resourceGroup, nil
}

func (rsg *ResourceGroupManager) CreateResourceGroup(ctx context.Context, subscriptionID string, resourceGroupName string, location string, tags map[string]*string) error {
	log := logging.GetLogger(ctx)

	clientOpts := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	}

	client, err := armresources.NewResourceGroupsClient(subscriptionID, rsg.token, clientOpts)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create resource group client", logging.WithError(err))
		return err
	}

	_, err = client.CreateOrUpdate(
		ctx,
		resourceGroupName,
		armresources.ResourceGroup{
			Location: &location,
			Tags:     tags,
		},
		nil,
	)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create resource group", logging.WithError(err))
		return err
	}

	log.InfoContext(ctx, fmt.Sprintf("Resource group %v created in location %v", resourceGroupName, location))
	return nil
}

func (rsg *ResourceGroupManager) GetResourceGroup(ctx context.Context, subscriptionID, name string) (*armresources.ResourceGroup, error) {
	log := logging.GetLogger(ctx)

	clientOpts := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	}

	client, err := armresources.NewResourceGroupsClient(subscriptionID, rsg.token, clientOpts)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create resource groups client", logging.WithError(err))
		return nil, err
	}

	resp, err := client.Get(ctx, name, nil)
	if err != nil {
		log.ErrorContext(ctx, "Failed to get resource groups", logging.WithError(err))
		return nil, err
	}

	return &resp.ResourceGroup, nil
}

func (rsg *ResourceGroupManager) DeleteResourceGroup(ctx context.Context, subscriptionID string, name string) error {
	log := logging.GetLogger(ctx)

	timeout, err := time.ParseDuration(rsg.config.PollingTimeoutDuration + "s")
	if err != nil {
		log.ErrorContext(ctx, "Failed to convert polling timeout to int", logging.WithError(err))
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	clientOpts := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	}

	client, err := armresources.NewResourceGroupsClient(subscriptionID, rsg.token, clientOpts)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create resource groups client", logging.WithError(err))
		return err
	}

	pollerResp, err := client.BeginDelete(ctx, name, nil)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create begin delete", logging.WithError(err))
		return err
	}

	_, err = pollerResp.PollUntilDone(ctx, nil)
	if err != nil {
		log.ErrorContext(ctx, "Failed to poll until done", logging.WithError(err))
		return err
	}

	if pollerResp != nil {
		log.InfoContext(ctx, fmt.Sprintf("Resource group %v deleted", name))
	} else {
		log.InfoContext(ctx, fmt.Sprintf("Resource group %v timeout, resource group not deleted", name))
	}
	return nil
}

func (rsg *ResourceGroupManager) ListResourceGroups(ctx context.Context, subscriptionID string) ([]*armresources.ResourceGroup, error) {
	log := logging.GetLogger(ctx)

	var resourceGroups []*armresources.ResourceGroup

	clientOpts := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	}

	client, err := armresources.NewResourceGroupsClient(subscriptionID, rsg.token, clientOpts)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create resource groups client", logging.WithError(err))
		return nil, err
	}

	pager := client.NewListPager(nil)

	for pager.More() {
		pageResp, err := pager.NextPage(ctx)
		if err != nil {
			log.ErrorContext(ctx, "Failed to get next page", logging.WithError(err))
			return nil, err
		}

		resourceGroups = append(resourceGroups, pageResp.Value...)
	}

	return resourceGroups, nil
}

func setConfig(rsg *ResourceGroupConfig) *ResourceGroupConfig {
	if rsg.PollingTimeoutDuration == "" {
		rsg.PollingTimeoutDuration = "30"
	}

	return &ResourceGroupConfig{
		PollingTimeoutDuration: rsg.PollingTimeoutDuration,
	}
}
