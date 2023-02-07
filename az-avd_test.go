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

var (
	ctx               context.Context = cloudy.StartContext()
	err               error
	tenantId          string
	clientId          string
	clientSecret      string
	subscriptionId    string
	resourceGroupName string
	avd               *AzureVirtualDesktop
	upn               string
)

func initAVD() error {
	_ = testutil.LoadEnv("test.env")
	tenantId = cloudy.ForceEnv("AZ_TENANT_ID", "")
	clientId = cloudy.ForceEnv("AZ_CLIENT_ID", "")
	clientSecret = cloudy.ForceEnv("AZ_CLIENT_SECRET", "")
	subscriptionId = cloudy.ForceEnv("AZ_SUBSCRIPTION_ID", "")
	resourceGroupName = cloudy.ForceEnv("AZ_RESOURCE_GROUP", "")
	upn = cloudy.ForceEnv("AZ_USER_PRINCIPAL_NAME", "")

	avd, err = NewAzureVirtualDesktop(ctx, AzureVirtualDesktopConfig{
		AzureCredentials: AzureCredentials{
			TenantID:     tenantId,
			ClientID:     clientId,
			ClientSecret: clientSecret,
			Region:       "usgovvirginia",
		},
		subscription: subscriptionId})

	return err
}

func TestRetrieveRegistrationToken(t *testing.T) {
	var (
		hostpools   []*armdesktopvirtualization.HostPool
		sessionHost *string
	)
	err = initAVD()
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

	if err == nil {
		sessionHost, err = avd.GetAvailableSessionHost(ctx, resourceGroupName, *firstHostpool.Name)
		assert.Nil(t, err)
		assert.NotEmpty(t, sessionHost)
	}

	if err == nil {
		err = avd.AssignSessionHost(ctx, resourceGroupName, *firstHostpool.Name, *sessionHost, upn)
		assert.Nil(t, err)
	}

	//if err == nil {
	//	err = avd.UnassignSessionHost(ctx, resourceGroupName, *firstHostpool.Name, sessionHost, upn)
	//	assert.Nil(t, err)
	//}
}

func TestAssignSessionHost(t *testing.T) {

	err = initAVD()
	assert.Nil(t, err)

	sessionHost := "col-avd-0-0"
	hostpoolname := "collider-avd-hp-01"

	err = avd.AssignSessionHost(ctx, resourceGroupName, hostpoolname, sessionHost, upn)
	assert.Nil(t, err)
}

func TestUnassignSessionHost(t *testing.T) {

	err = initAVD()
	assert.Nil(t, err)

	sessionHost := "col-avd-0-0"
	hostpoolname := "collider-avd-hp-01"

	err = avd.DeleteSessionHost(ctx, resourceGroupName, hostpoolname, sessionHost, upn)
	assert.Nil(t, err)
}

func TestAssignUsertoRoles(t *testing.T) {
	// Desktop Virtualization User 1d18fff3-a72a-46b5-b4a9-0b38a3cd7e63
	// Virtual Machine User Login fb879df8-f326-4884-b1cf-06f3ad86be52
	// c37460e9-aef0-4add-91e2-3e80dfbc73ed
	err = initAVD()
	assert.Nil(t, err)

	roleid := "1d18fff3-a72a-46b5-b4a9-0b38a3cd7e63"
	userobjectid := "c37460e9-aef0-4add-91e2-3e80dfbc73ed"

	err = avd.AssignRolesToUser(ctx, resourceGroupName, roleid, userobjectid)
	assert.Nil(t, err)
}
