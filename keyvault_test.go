package cloudyazure

import (
	"context"
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/secrets"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestKeyVault(t *testing.T) {

	testutil.LoadEnv("test.env")
	tenantID := cloudy.ForceEnv("TenantID", "")
	ClientID := cloudy.ForceEnv("ClientID", "")
	ClientSecret := cloudy.ForceEnv("ClientSecret", "")

	creds := AzureCredentials{
		TenantID:     tenantID,
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
	}
	vaultURL := "https://gokeyvault.vault.usgovcloudapi.net/"

	kv, err := NewKeyVault(context.Background(), vaultURL, creds)
	assert.Nil(t, err)

	secrets.SecretsTest(t, context.Background(), kv)
}

func TestKeyVaultDynamic(t *testing.T) {

}
