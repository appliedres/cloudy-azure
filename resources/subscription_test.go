package resources

import (
	"os"
	"testing"

	"github.com/appliedres/cloudy"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/stretchr/testify/assert"
)

func InitSubscription() (*SubscriptionManager, error) {
	err := cloudy.LoadEnv("../.env.local")
	if err != nil {
		return nil, err
	}

	ctx := cloudy.StartContext()

	clientId := os.Getenv("AZ_CLIENT_ID")
	clientSecret := os.Getenv("AZ_CLIENT_SECRET")
	tenantId := os.Getenv("AZ_TENANT_ID")
	subscriptionId := os.Getenv("AZ_SUBSCRIPTION_ID")

	creds := cloudyazure.AzureCredentials{
		TenantID:       tenantId,
		ClientID:       clientId,
		ClientSecret:   clientSecret,
		Region:         "usgovvirginia",
		SubscriptionID: subscriptionId,
	}

	config := SubscriptionConfig{
		PollingTimeoutDuration: "30",
	}

	sub, err := NewSubscriptionManager(ctx, &config, &creds)
	if err != nil {
		return nil, err
	}
	return sub, err
}

func TestCreateSubscription(t *testing.T) {
	sub, err := InitSubscription()
	assert.NoError(t, err)

	ctx := cloudy.StartContext()

	err = sub.CreateSubscription(ctx, "TestCreateSubscription")
	assert.NoError(t, err)
}

func TestListSubscriptions(t *testing.T) {
	sub, err := InitSubscription()
	assert.NoError(t, err)

	ctx := cloudy.StartContext()

	subs, err := sub.ListSubscriptions(ctx)
	assert.NoError(t, err)
	assert.NotEmpty(t, subs)
}
