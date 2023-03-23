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
	env := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(env)

	tenantID := env.Force("AZ_TENANT_ID")
	ClientID := env.Force("AZ_CLIENT_ID")
	ClientSecret := env.Force("AZ_CLIENT_SECRET")
	subscriptionId = env.Force("AZ_SUBSCRIPTION_ID")
	vaultUrl = env.Force("AZ_VAULT_URL")

	ctx := cloudy.StartContext()

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
