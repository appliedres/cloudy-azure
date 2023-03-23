package cloudyazure

import (
	"log"
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
)

func TestBlobFileshare(t *testing.T) {

	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")
	env := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(env)

	ctx = cloudy.StartContext()
	tenantID := env.Force("AZ_TENANT_ID")
	ClientID := env.Force("AZ_CLIENT_ID")
	ClientSecret := env.Force("AZ_CLIENT_SECRET")
	subscriptionId := env.Force("AZ_SUBSCRIPTION_ID")
	vaultUrl = env.Force("AZ_VAULT_URL")

	account := env.Force("FS_ACCOUNT")
	resourceGroup := env.Force("RG_FILESHARE")

	creds := AzureCredentials{
		TenantID:     tenantID,
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
	}

	bfa, err := NewBlobFileShare(ctx, &BlobFileShare{
		Credentials:        creds,
		StorageAccountName: account,
		ResourceGroupName:  resourceGroup,
		SubscriptionID:     subscriptionId,
	})
	if err != nil {
		log.Fatal(err)
		// t.FailNow()
	}

	testutil.TestFileShareStorageManager(t, bfa, "file-storage-test")

	testutil.TestFileShareStorageManager(t, bfa, "Test-Share")
}
