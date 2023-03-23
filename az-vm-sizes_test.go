package cloudyazure

import (
	// "crypto/x509"
	// "encoding/pem"
	// "fmt"
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
	// "golang.org/x/crypto/ssh"
)

func TestLoad(t *testing.T) {
	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")

	newEnv := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(newEnv)
	ctx := cloudy.StartContext()

	vaultUrl := newEnv.Force("AZ_VAULT_URL")
	creds := GetAzureCredentialsFromEnv(cloudy.DefaultEnvironment)

	kve, _ := NewKeyVaultEnvironmentService(ctx, vaultUrl, creds, "")

	env := cloudy.NewTieredEnvironment(
		cloudy.NewTestFileEnvironmentService(),
		kve,
	)

	ctx = cloudy.StartContext()
	tenantID, _ := env.Get("AZ_TENANT_ID")
	ClientID, _ := env.Get("AZ_CLIENT_ID")
	ClientSecret, _ := env.Get("AZ_CLIENT_SECRET")
	SubscriptionID, _ := env.Get("AZ_SUBSCRIPTION_ID")
	resourceGroup, _ := env.Get("AZ_RESOURCE_GROUP")
	vNet, _ := env.Get("AZ_VNET")
	subnet, _ := env.Get("AZ_SUBNET")
	nsgName, _ := env.Get("AZ_NSG_NAME")
	imageGallery, _ := env.Get("VMC_AZ_SOURCE_IMAGE_GALLERY_NAME")

	vmc, err := NewAzureVMController(ctx, &AzureVMControllerConfig{
		AzureCredentials: AzureCredentials{
			TenantID:     tenantID,
			ClientID:     ClientID,
			ClientSecret: ClientSecret,
			Region:       "usgovvirginia",
		},

		SubscriptionID: SubscriptionID,

		ResourceGroup:            resourceGroup,
		NetworkResourceGroup:     resourceGroup,
		SourceImageGalleryName:   imageGallery,
		Vnet:                     vNet,
		AvailableSubnets:         []string{subnet},
		NetworkSecurityGroupName: nsgName,
		NetworkSecurityGroupID:   "NOT SET",
		VaultURL:                 vaultUrl,
	})
	assert.Nil(t, err)

	cache := &AzureVMSizeCache{}
	loaded := cache.Load(ctx, vmc)
	assert.NotNil(t, cache)
	assert.Nil(t, loaded)
}

func TestMerge(t *testing.T) {
	// NOOP - Merge has no implementation
}
