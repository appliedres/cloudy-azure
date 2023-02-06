package cloudyazure

import (
	"fmt"
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestAllVMStatus(t *testing.T) {
	ctx := cloudy.StartContext()
	_ = testutil.LoadEnv("test.env")

	tenantID := cloudy.ForceEnv("AZ_TENANT_ID", "")
	ClientID := cloudy.ForceEnv("AZ_CLIENT_ID", "")
	ClientSecret := cloudy.ForceEnv("AZ_CLIENT_SECRET", "")
	resourceGroup := cloudy.ForceEnv("AZ_RESOURCE_GROUP", "")
	SubscriptionID := cloudy.ForceEnv("AZ_SUBSCRIPTION_ID", "")

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
