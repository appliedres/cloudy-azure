package cloudyazure

// Import key modules.
import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization"
	"github.com/appliedres/cloudy"

	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

type Config struct {
	Upn                             string
	UserObjectId                    string
	SessionHost                     string
	HostPool                        string
	DesktopVirtualizationUserRoleId string
	VirtualMachineUserLoginRoleId   string
	ResourceGroupName               string
}

var (
	ctx            context.Context
	err            error
	tenantId       string
	clientId       string
	clientSecret   string
	subscriptionId string
	avd            *AzureVirtualDesktop
	testConfig     Config
	vaultUrl       string
)

func initAVD() error {
	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")
	env := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(env)

	ctx = cloudy.StartContext()
	tenantId = env.Force("AZ_TENANT_ID")
	clientId = env.Force("AZ_CLIENT_ID")
	clientSecret = env.Force("AZ_CLIENT_SECRET")
	subscriptionId = env.Force("AZ_SUBSCRIPTION_ID")
	vaultUrl = env.Force("AZ_VAULT_URL")

	avd, err = NewAzureVirtualDesktop(ctx, AzureVirtualDesktopConfig{
		AzureCredentials: AzureCredentials{
			TenantID:     tenantId,
			ClientID:     clientId,
			ClientSecret: clientSecret,
			Region:       "usgovvirginia",
		},
		subscription: subscriptionId})
	if err != nil {
		return err
	}

	cred, err := GetAzureClientSecretCredential(avd.config.AzureCredentials)
	if err != nil {
		return err
	}

	client, err := azsecrets.NewClient(vaultUrl, cred, nil)
	if err != nil {
		return err
	}

	res, err := client.GetSecret(ctx, "avd-test-desktop-virtualization-user-role-id", "", nil)
	if err != nil {
		return err
	}
	testConfig.DesktopVirtualizationUserRoleId = *res.Value

	res, err = client.GetSecret(ctx, "avd-test-hostpool", "", nil)
	if err != nil {
		return err
	}
	testConfig.HostPool = *res.Value

	res, err = client.GetSecret(ctx, "avd-test-resource-group", "", nil)
	if err != nil {
		return err
	}
	testConfig.ResourceGroupName = *res.Value

	res, err = client.GetSecret(ctx, "avd-test-session-host", "", nil)
	if err != nil {
		return err
	}
	testConfig.SessionHost = *res.Value

	res, err = client.GetSecret(ctx, "avd-test-upn", "", nil)
	if err != nil {
		return err
	}
	testConfig.Upn = *res.Value

	res, err = client.GetSecret(ctx, "avd-test-user-object-id", "", nil)
	if err != nil {
		return err
	}
	testConfig.UserObjectId = *res.Value

	res, err = client.GetSecret(ctx, "avd-test-user-object-id", "", nil)
	if err != nil {
		return err
	}
	testConfig.VirtualMachineUserLoginRoleId = *res.Value

	return nil
}

func TestRetrieveRegistrationToken(t *testing.T) {
	var (
		hostpools   []*armdesktopvirtualization.HostPool
		sessionHost *string
		regToken    *string
	)
	err = initAVD()
	assert.Nil(t, err)

	hostpools, err = avd.ListHostPools(ctx, testConfig.ResourceGroupName)
	assert.Nil(t, err)
	assert.NotZero(t, len(hostpools))

	for i := 1; i < len(hostpools); i++ {
		sessionHosts, err := avd.ListSessionHosts(ctx, testConfig.ResourceGroupName, *hostpools[i].Name)
		assert.GreaterOrEqual(t, len(sessionHosts), 0)
		assert.Nil(t, err)
	}

	firstHostpool, err := avd.FindFirstAvailableHostPool(ctx, testConfig.ResourceGroupName, testConfig.Upn)
	assert.Nil(t, err)
	assert.NotNil(t, firstHostpool)
	assert.NotEmpty(t, firstHostpool)

	if err == nil && firstHostpool != nil {
		regToken, err = avd.RetrieveRegistrationToken(ctx, testConfig.ResourceGroupName, *firstHostpool.Name)
		assert.Nil(t, err)
		assert.NotNil(t, regToken)
		assert.NotEmpty(t, regToken)
	}

	if err == nil && firstHostpool != nil {
		sessionHost, err = avd.GetAvailableSessionHost(ctx, testConfig.ResourceGroupName, *firstHostpool.Name)
		assert.Nil(t, err)
		assert.NotNil(t, sessionHost)
		assert.NotEmpty(t, sessionHost)
	}

	if err == nil && firstHostpool != nil && sessionHost != nil {
		err = avd.AssignSessionHost(ctx, testConfig.ResourceGroupName, *firstHostpool.Name, *sessionHost, testConfig.Upn)
		assert.Nil(t, err)
	}
}

func TestAssignSessionHost(t *testing.T) {
	err = initAVD()
	assert.Nil(t, err)

	err = avd.AssignSessionHost(ctx, testConfig.ResourceGroupName, testConfig.HostPool, testConfig.SessionHost, testConfig.Upn)
	assert.Nil(t, err)
}

func TestDeleteSessionHost(t *testing.T) {
	err = initAVD()
	assert.Nil(t, err)

	err = avd.DeleteSessionHost(ctx, testConfig.ResourceGroupName, testConfig.HostPool, testConfig.SessionHost)
	assert.Nil(t, err)
}

func TestDeleteUserSession(t *testing.T) {
	err = initAVD()
	assert.Nil(t, err)

	err = avd.DeleteUserSession(ctx, testConfig.ResourceGroupName, testConfig.HostPool, testConfig.SessionHost, testConfig.Upn)
	assert.Nil(t, err)
}

func TestDisconnecteUserSession(t *testing.T) {
	err = initAVD()
	assert.Nil(t, err)

	err = avd.DisconnecteUserSession(ctx, testConfig.ResourceGroupName, testConfig.HostPool, testConfig.SessionHost, testConfig.Upn)
	assert.Nil(t, err)
}

func TestAssignUsertoRoles(t *testing.T) {
	err = initAVD()
	assert.Nil(t, err)

	err = avd.AssignRoleToUser(ctx, testConfig.ResourceGroupName, testConfig.DesktopVirtualizationUserRoleId, testConfig.UserObjectId)
	assert.Nil(t, err)

	err = avd.AssignRoleToUser(ctx, testConfig.ResourceGroupName, testConfig.VirtualMachineUserLoginRoleId, testConfig.UserObjectId)
	assert.Nil(t, err)
}
