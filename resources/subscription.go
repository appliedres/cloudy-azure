package resources

import (
	"bytes"
	"context"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy/logging"
)

type SubscriptionConfig struct {
	TenantId               string
	PollingTimeoutDuration string
}

type SubscriptionManager struct {
	credentials *cloudyazure.AzureCredentials
	token       azcore.TokenCredential
	config      *SubscriptionConfig
	client      *armsubscriptions.Client
}

func NewSubscriptionManager(ctx context.Context, config *SubscriptionConfig, creds *cloudyazure.AzureCredentials) (*SubscriptionManager, error) {
	log := logging.GetLogger(ctx)

	subscriptionManager := &SubscriptionManager{
		credentials: creds,
	}

	subscriptionManager.config = setSubConfig(config, creds)

	cred, err := cloudyazure.NewAzureCredentials(creds)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create azure token", logging.WithError(err))
		return nil, err
	}

	subscriptionManager.token = cred

	clientOpts := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	}
	client, err := armsubscriptions.NewClient(subscriptionManager.token, clientOpts)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create subscription client", logging.WithError(err))
		return nil, err
	}

	subscriptionManager.client = client
	return subscriptionManager, nil
}

func (sm *SubscriptionManager) CreateSubscription(ctx context.Context, subscriptionName string) error {
	log := logging.GetLogger(ctx)
	url := "https://management.usgovcloudapi.net/providers/Microsoft.Subscription/subscriptions?api-version=2020-09-01"

	jsonData := []byte(`{
        "displayName": "` + subscriptionName + `",
        "tenantId": "` + sm.config.TenantId + `"
    }`)

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.ErrorContext(ctx, "Failed to create request", logging.WithError(err))
		return err
	}

	token, err := sm.token.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{"https://management.usgovcloudapi.net/.default"}})
	if err != nil {
		log.ErrorContext(ctx, "Failed to get token", logging.WithError(err))
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token.Token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create subscription", logging.WithError(err))
		return err
	}
	defer resp.Body.Close()

	log.InfoContext(ctx, "Subscription creation response status", resp.Status, nil)
	return nil
}

func (sm *SubscriptionManager) ListSubscriptions(ctx context.Context) ([]*armsubscriptions.Subscription, error) {
	var subscriptions []*armsubscriptions.Subscription
	log := logging.GetLogger(ctx)
	pager := sm.client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			log.ErrorContext(ctx, "Failed to retrieve subscription page", logging.WithError(err))
			return nil, err
		}
		for _, subscription := range page.Value {
			subscriptions = append(subscriptions, subscription)
			log.DebugContext(ctx, "Subscription ID: ", *subscription.SubscriptionID, nil)
			log.DebugContext(ctx, "Subscription Name: ", *subscription.DisplayName, nil)
		}
	}
	return subscriptions, nil
}

func setSubConfig(config *SubscriptionConfig, credentials *cloudyazure.AzureCredentials) *SubscriptionConfig {
	if config == nil {
		config = &SubscriptionConfig{}
	}

	if config.PollingTimeoutDuration == "" {
		config.PollingTimeoutDuration = "30"
	}

	if config.TenantId == "" {
		config.TenantId = credentials.TenantID
	}

	return config
}
