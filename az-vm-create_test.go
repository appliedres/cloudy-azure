package cloudyazure

import (
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	cloudyvm "github.com/appliedres/cloudy/vm"
	"github.com/stretchr/testify/assert"
)

func TestVMCreate(t *testing.T) {
	ctx := cloudy.StartContext()
	testutil.LoadEnv("test.env")

	tenantID := cloudy.ForceEnv("TenantID", "")
	ClientID := cloudy.ForceEnv("ClientID", "")
	ClientSecret := cloudy.ForceEnv("ClientSecret", "")
	SubscriptionID := cloudy.ForceEnv("SUBSCRIPTION_ID", "")

	vmc, err := NewAzureVMController(ctx, &AzureVMControllerConfig{
		AzureCredentials: AzureCredentials{
			TenantID:     tenantID,
			ClientID:     ClientID,
			ClientSecret: ClientSecret,
			Region:       "usgovvirginia",
		},
		SubscriptionID: SubscriptionID,

		ResourceGroup:            "go-on-azure",
		NetworkResourceGroup:     "go-on-azure",
		SourceImageGalleryName:   "testimagegallery",
		Vnet:                     "go-on-azure-vmVNET",
		AvailableSubnets:         []string{"go-on-azure-vmSubnet"},
		NetworkSecurityGroupName: "go-on-azure-vmNSG",
		NetworkSecurityGroupID:   "NOT SET",
		SaltCmd:                  "TESTSALT",
		VaultURL:                 "https://gokeyvault.vault.usgovcloudapi.net/",
	})
	assert.Nil(t, err)

	// vmc.GetVMSize(ctx, "asdfaf")

	cache := &AzureVMSizeCache{}
	cache.Load(ctx, vmc)

	/*
	   "imageReference": {
	   	"publisher": "canonical",
	   	"offer": "ubuntuserver",
	   	"sku": "19.04",
	   	"version": "latest"
	   },
	*/
	vmConfig := &cloudyvm.VirtualMachineConfiguration{
		ID:           "uvm-gotest",
		Name:         "uvm-gotest",
		Size:         "Standard_DS1_v2",
		OSType:       "linux",
		Image:        "canonical::ubuntuserver::19.04",
		ImageVersion: "latest",
		Credientials: cloudyvm.Credientials{
			AdminUser:     "testadmin",
			AdminPassword: "TestPassword12#$",
		},
	}

	// Test subnet
	subnet, err := vmc.FindBestSubnet(ctx, []string{"go-on-azure-vmSubnet"})
	assert.Nil(t, err)
	assert.Equal(t, "go-on-azure-vmSubnet", subnet)

	// Test Create NIC
	err = vmc.CreateNIC(ctx, vmConfig, subnet)
	assert.Nil(t, err)
	assert.NotNil(t, vmConfig.PrimaryNetwork)
	assert.NotNil(t, vmConfig.PrimaryNetwork.ID)
	assert.NotNil(t, vmConfig.PrimaryNetwork.Name)
	assert.NotNil(t, vmConfig.PrimaryNetwork.PrivateIP)

	defer vmc.DeleteNIC(ctx, vmConfig.PrimaryNetwork.Name)

	// Test Create
	err = vmc.CreateLinuxVirtualMachine(ctx, vmConfig)
	assert.Nil(t, err)

	if err == nil {
		err = vmc.DeleteVM(ctx, vmConfig.Name)
		assert.Nil(t, err)
	}

}
