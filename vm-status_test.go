package cloudyazure

import (
	"fmt"
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestVMStatus(t *testing.T) {
	ctx := cloudy.StartContext()
	_ = testutil.LoadEnv("test.env")

	tenantID := cloudy.ForceEnv("TenantID", "")
	ClientID := cloudy.ForceEnv("ClientID", "")
	ClientSecret := cloudy.ForceEnv("ClientSecret", "")
	resourceGroup := cloudy.ForceEnv("resourceGroup", "")
	SubscriptionID := cloudy.ForceEnv("SUBSCRIPTION_ID", "")

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
		fmt.Printf("%v\n", vm.Name)
	}

}
