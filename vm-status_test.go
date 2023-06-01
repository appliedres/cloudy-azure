package cloudyazure

import (
	"fmt"
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestAllVMStatus(t *testing.T) {
	testutil.MustSetTestEnv()
	env := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "USER_API")
	cloudy.SetDefaultEnvironment(env)

	ctx := cloudy.StartContext()

	// _, err := cloudyvm.VmControllers.NewFromEnv(vmApi, "DRIVER")

	tenantID := env.Force("AZ_TENANT_ID", "")
	ClientID := env.Force("AZ_CLIENT_ID", "")
	ClientSecret := env.Force("AZ_CLIENT_SECRET", "")

	vmApi := env.Segment("VMC")
	resourceGroup := vmApi.Force("AZ_RESOURCE_GROUP", "")
	SubscriptionID := vmApi.Force("AZ_SUBSCRIPTION_ID", "")

	vmc, err := NewAzureVMController(ctx, &AzureVMControllerConfig{
		AzureCredentials: AzureCredentials{
			TenantID:     tenantID,
			ClientID:     ClientID,
			ClientSecret: ClientSecret,
			Region:       DefaultRegion,
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

	sizes, err := vmc.GetVMSizes(ctx)
	assert.Nil(t, err)

	assert.NotNil(t, sizes)
	for _, size := range sizes {
		fmt.Printf("%v -- %v\n", resourceGroup, size)
	}

	limits, err := vmc.GetLimits(ctx)
	assert.Nil(t, err)

	assert.NotNil(t, limits)
	for _, limit := range limits {
		fmt.Printf("%v -- %v\n", resourceGroup, limit)
	}

}
