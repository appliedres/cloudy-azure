package cloudyazure

// Import key modules.
import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization"
	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestFetchHPRegKey(t *testing.T) {

	var (
		ctx       context.Context = cloudy.StartContext()
		hostpools []*armdesktopvirtualization.HostPool
		err       error
	)

	_ = testutil.LoadEnv("test.env")

	tenantId := cloudy.ForceEnv("AZ_TENANT_ID", "")
	clientId := cloudy.ForceEnv("AZ_CLIENT_ID", "")
	clientSecret := cloudy.ForceEnv("AZ_CLIENT_SECRET", "")
	subscriptionId := cloudy.ForceEnv("AZ_SUBSCRIPTION_ID", "")
	resourceGroupName := cloudy.ForceEnv("AZ_RESOURCE_GROUP", "")
	upn := cloudy.ForceEnv("AZ_USER_PRINCIPAL_NAME", "")

	avd, err := NewAzureVirtualDesktop(ctx, AzureVirtualDesktopConfig{
		AzureCredentials: AzureCredentials{
			TenantID:     tenantId,
			ClientID:     clientId,
			ClientSecret: clientSecret,
			Region:       "usgovvirginia",
		},
		subscription: subscriptionId})
	assert.Nil(t, err)

	hostpools, err = avd.ListHostPools(ctx, resourceGroupName)
	assert.Nil(t, err)
	assert.NotZero(t, len(hostpools))

	for i := 1; i < len(hostpools); i++ {
		sessionHosts, err := avd.ListSessionHosts(ctx, resourceGroupName, *hostpools[i].Name)
		assert.GreaterOrEqual(t, len(sessionHosts), 0)
		assert.Nil(t, err)
	}

	firstHostpool, err := avd.FindFirstAvailableHostPool(ctx, resourceGroupName, upn)
	assert.Nil(t, err)
	assert.NotEmpty(t, firstHostpool)

	if err == nil {
		regToken, err := avd.RetrieveRegistrationToken(ctx, resourceGroupName, *firstHostpool.Name)
		assert.Nil(t, err)
		assert.NotEmpty(t, regToken)
	}

}
