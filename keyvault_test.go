package cloudyazure

import (
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/secrets"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestKeyVault(t *testing.T) {

	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")

	ctx := cloudy.StartContext()

	tenantID := cloudy.ForceEnv("KEYVAULT_AZ_TENANT_ID", "")
	ClientID := cloudy.ForceEnv("KEYVAULT_AZ_CLIENT_ID", "")
	ClientSecret := cloudy.ForceEnv("KEYVAULT_AZ_CLIENT_SECRET", "")

	creds := AzureCredentials{
		TenantID:     tenantID,
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
	}
	vaultURL := cloudy.ForceEnv("AZ_VAULT_URL", "")

	kv, err := NewKeyVault(ctx, vaultURL, creds)
	assert.Nil(t, err)

	secrets.SecretsTest(t, ctx, kv)
}

func TestKeyVaultTemp(t *testing.T) {
	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")

	ctx := cloudy.StartContext()

	tenantID := cloudy.ForceEnv("KEYVAULT_AZ_TENANT_ID", "")
	ClientID := cloudy.ForceEnv("KEYVAULT_AZ_CLIENT_ID", "")
	ClientSecret := cloudy.ForceEnv("KEYVAULT_AZ_CLIENT_SECRET", "")

	creds := AzureCredentials{
		TenantID:     tenantID,
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
	}
	vaultURL := cloudy.ForceEnv("AZ_VAULT_URL", "")

	provider, err := NewKeyVault(ctx, vaultURL, creds)
	assert.Nil(t, err)

	secStr2, err := provider.GetSecret(ctx, "mytest")
	assert.Nil(t, err)
	assert.Equal(t, secStr2, "test")

	err = provider.SaveSecret(ctx, "mytest2", "test2")
	assert.Nil(t, err)

}
