package resources

import (
	"os"
	"testing"
	"time"

	"github.com/appliedres/cloudy"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/stretchr/testify/assert"
)

func InitRsg() (*ResourceGroupManager, error) {
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

	config := ResourceGroupConfig{
		PollingTimeoutDuration: "30",
	}

	rsg, err := NewResourceGroupManager(ctx, &config, &creds)
	return rsg, err
}

func TestCreateRsg(t *testing.T) {
	createdByTag := "TestCreateRsg"
	rsg, err := InitRsg()
	assert.NoError(t, err)
	assert.NotNil(t, rsg)

	ctx := cloudy.StartContext()

	tags := make(map[string]*string)
	tags["CreatedBy"] = &createdByTag

	err = rsg.CreateResourceGroup(ctx, rsg.credentials.SubscriptionID, "TestCreateRsg", rsg.credentials.Region, tags)
	assert.NoError(t, err)

	time.Sleep(3 * time.Second)

	rg, err := rsg.GetResourceGroup(ctx, rsg.credentials.SubscriptionID, "TestCreateRsg")
	assert.NoError(t, err)
	assert.NotNil(t, rg)
	assert.Equal(t, "TestCreateRsg", *rg.Name)
	assert.Equal(t, rsg.credentials.Region, *rg.Location)
	assert.Equal(t, "Microsoft.Resources/resourceGroups", *rg.Type)
	assert.Equal(t, "Succeeded", *rg.Properties.ProvisioningState)
	assert.Equal(t, createdByTag, *rg.Tags["CreatedBy"])

	rgs, err := rsg.ListResourceGroups(ctx, rsg.credentials.SubscriptionID)
	assert.NoError(t, err)
	assert.NotNil(t, rgs)
	assert.Greater(t, len(rgs), 0)

	// err = rsg.DeleteResourceGroup(ctx, rsg.credentials.SubscriptionID, "TestCreateRsg")
	// assert.NoError(t, err)
}
