package cloudyazure

import (
	"fmt"
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestAllVMStatus(t *testing.T) {
	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")
	env := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(env)

	ctx = cloudy.StartContext()
	tenantID := env.Force("AZ_TENANT_ID")
	ClientID := env.Force("AZ_CLIENT_ID")
	ClientSecret := env.Force("AZ_CLIENT_SECRET")
	SubscriptionID := env.Force("AZ_SUBSCRIPTION_ID")

	vaultUrl = env.Force("AZ_VAULT_URL")

	resourceGroup := env.Force("AZ_RESOURCE_GROUP")

	vmc, err := NewAzureVMController(ctx, &AzureVMControllerConfig{
		AzureCredentials: AzureCredentials{
			TenantID:     tenantID,
			ClientID:     ClientID,
			ClientSecret: ClientSecret,
		},
		SubscriptionID: SubscriptionID,
		ResourceGroup:  resourceGroup,
	})

	assert.Nil(t, err)

	all, err := vmc.ListAll(ctx)
	assert.Nil(t, err)

	assert.NotNil(t, all)
	for _, vm := range all {
		fmt.Printf("%v -- %s -- %s\n", resourceGroup, vm.Name, vm.PowerState)
	}

}
