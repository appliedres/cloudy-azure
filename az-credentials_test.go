package cloudyazure

import (
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestReadFromEnv(t *testing.T) {
	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")
	env := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(env)

	ctx = cloudy.StartContext()
	tenantId = env.Force("AZ_TENANT_ID")
	clientId = env.Force("AZ_CLIENT_ID")
	clientSecret = env.Force("AZ_CLIENT_SECRET")
	vaultUrl = env.Force("AZ_REGION")

	azureCredentialLoader := AzureCredentialLoader{}
	azureCredentialInterface := azureCredentialLoader.ReadFromEnv(env)
	assert.NotNil(t, azureCredentialInterface)

	actual := azureCredentialInterface.(AzureCredentials)
	assert.NotNil(t, actual)
	assert.EqualValues(t, tenantId, actual.TenantID)
	assert.EqualValues(t, clientId, actual.ClientID)
	assert.EqualValues(t, clientSecret, actual.ClientSecret)
	assert.EqualValues(t, vaultUrl, actual.Region)
}
