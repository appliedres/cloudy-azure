package avd

import (
	"os"
	"testing"
	"time"

	"github.com/appliedres/cloudy"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/stretchr/testify/assert"
)

func initAVDManager() (*AzureVirtualDesktopManager, error) {
	ctx := cloudy.StartContext()

	// TODO: modify an existing config api instead of env vars
	credentials := &cloudyazure.AzureCredentials{
		TenantID:       os.Getenv("AZ_TENANT_ID"),
		ClientID:       os.Getenv("AZ_CLIENT_ID"),
		ClientSecret:   os.Getenv("AZ_CLIENT_SECRET"),
		ResourceGroup:  os.Getenv("AZ_RESOURCE_GROUP"),
		SubscriptionID: os.Getenv("AZ_SUBSCRIPTION_ID"),
		Region:         os.Getenv("AZ_REGION"),
		Cloud:          os.Getenv("AZ_CLOUD"),
	}

	// TODO: modify an existing config api instead of env vars
	rdmConfig := &cloudyazure.AzureVirtualDesktopConfig{
		AvdUsersGroupId:              os.Getenv("AZ_AVD_USERS_GROUP_ID"),
		DomainName:                   os.Getenv("AZ_AVD_DOMAIN_NAME"),
		DomainUser:                   os.Getenv("AZ_AVD_DOMAIN_USER"),
		DomainPass:                   os.Getenv("AZ_AVD_DOMAIN_PASS"),
		Region:                       os.Getenv("AZ_AVD_REGION"),
		DesktopApplicationUserRoleID: os.Getenv("AZ_AVD_DESKTOP_APPLICATION_USER_ROLE_ID"),
		AVDUserGroupID:               os.Getenv("AZ_AVD_AVD_USER_GROUP_ID"),
		UriEnv:                       os.Getenv("AZ_AVD_URI_ENV"),
	}

	avdm, err := NewAzureVirtualDesktopManager(ctx, credentials, rdmConfig)
	if err != nil {
		return nil, err
	}

	return avdm, err
}

func TestValidAVDManagerHPs(t *testing.T) {
	avdm, err := initAVDManager()
	assert.NoError(t, err)
	assert.NotNil(t, avdm)

	hp, err := avdm.CreateHostPool(ctx, avdm.credentials.ResourceGroup, "BILL-TEST1")
	assert.NoError(t, err)
	assert.NotNil(t, hp)

	time.Sleep(3 * time.Second)

	token, err := avdm.RetrieveRegistrationToken(ctx, avdm.credentials.ResourceGroup, *hp.Name)
	assert.NoError(t, err)
	assert.NotNil(t, token)

	hp1, err := avdm.UpdateHostPoolRegToken(ctx, avdm.credentials.ResourceGroup, *hp.Name)
	assert.NoError(t, err)
	assert.NotNil(t, hp1)

	time.Sleep(3 * time.Second)

	err = avdm.DeleteHostPool(ctx, avdm.credentials.ResourceGroup, *hp.Name)
	assert.NoError(t, err)
}
