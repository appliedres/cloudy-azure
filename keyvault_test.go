package cloudyazure

import (
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/secrets"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestKeyVault(t *testing.T) {

	_ = testutil.LoadEnv("test.env")

	ctx := cloudy.StartContext()

	tenantID := cloudy.ForceEnv("TenantID", "")
	ClientID := cloudy.ForceEnv("ClientID", "")
	ClientSecret := cloudy.ForceEnv("ClientSecret", "")

	creds := AzureCredentials{
		TenantID:     tenantID,
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
	}
	vaultURL := "https://gokeyvault.vault.usgovcloudapi.net/"

	kv, err := NewKeyVault(ctx, vaultURL, creds)
	assert.Nil(t, err)

	secrets.SecretsTest(t, ctx, kv)
}

func TestKeyVaultDynamic(t *testing.T) {

}
