package cloudyazure

import (
	"fmt"
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

// TODO: finish updating to env manager, test
func TestAllVMStatus(t *testing.T) {
	testutil.MustSetTestEnv()
	em := cloudy.GetDefaultEnvManager()
	em.LoadSources("test")

	ctx := cloudy.StartContext()

	// _, err := cloudyvm.VmControllers.NewFromEnvMgr(vmApi, "DRIVER")

	tenantID := em.GetVar("AZ_TENANT_ID", "")
	ClientID := em.GetVar("AZ_CLIENT_ID", "")
	ClientSecret := em.GetVar("AZ_CLIENT_SECRET", "")

	resourceGroup := em.GetVar("VMC_AZ_RESOURCE_GROUP", "")
	SubscriptionID := em.GetVar("VMC_AZ_SUBSCRIPTION_ID", "")

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
