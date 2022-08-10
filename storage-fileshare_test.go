package cloudyazure

import (
	"log"
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
)

func TestBlobFileshare(t *testing.T) {
	ctx := cloudy.StartContext()
	testutil.LoadEnv("test.env")
	tenantID := cloudy.ForceEnv("TenantID", "")
	ClientID := cloudy.ForceEnv("ClientID", "")
	ClientSecret := cloudy.ForceEnv("ClientSecret", "")
	account := cloudy.ForceEnv("fsAccount", "")
	resourceGroup := cloudy.ForceEnv("rgFileshare", "")
	subscriptionId := cloudy.ForceEnv("SUBSCRIPTION_ID", "")

	creds := AzureCredentials{
		TenantID:     tenantID,
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
	}

	bfa, err := NewBlobFileShare(ctx, &BlobFileShare{
		Credentials:        creds,
		StorageAccountName: account,
		// ContainerName:      "test-share",
		ResourceGroupName: resourceGroup,
		SubscriptionID:    subscriptionId,
	})
	if err != nil {
		log.Fatal(err)
		// t.FailNow()
	}

	testutil.TestFileShareStorageManager(t, bfa)
}
