package cloudyazure

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	cloudyvm "github.com/appliedres/cloudy/vm"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
)

func TestLinuxVMCreate(t *testing.T) {
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

	sshPublicKeySecretName := "VMSSHPublicKey"
	keyVault, err := NewKeyVault(ctx, vmc.Config.VaultURL, vmc.Config.AzureCredentials)
	assert.Nil(t, err)

	sshPublicKey, err := keyVault.GetSecret(ctx, sshPublicKeySecretName)
	assert.Nil(t, err)
	assert.NotNil(t, sshPublicKey)

	sshPrivateKeySecretName := "VMSSHPrivateKey"
	sshPrivateKey, err := keyVault.GetSecret(ctx, sshPrivateKeySecretName)
	assert.Nil(t, err)
	assert.NotNil(t, sshPrivateKey)

	/*
	   "imageReference": {
	   	"publisher": "canonical",
	   	"offer": "ubuntuserver",
	   	"sku": "19.04",
	   	"version": "latest"
	   },
	*/
	vmConfig := &cloudyvm.VirtualMachineConfiguration{
		ID:   "uvm-gotest",
		Name: "uvm-gotest",
		SizeRequest: &cloudyvm.VmSizeRequest{
			SpecificSize: "Standard_DS1_v2",
		},
		OSType:       "linux",
		Image:        "canonical::ubuntuserver::19.04",
		ImageVersion: "latest",
		Credientials: cloudyvm.Credientials{
			AdminUser:     "testadmin",
			AdminPassword: "TestPassword12#$",
			SSHKey:        sshPublicKey,
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
		block, _ := pem.Decode([]byte(sshPrivateKey))
		assert.NotNil(t, block)

		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		assert.Nil(t, err)

		signer, err := ssh.NewSignerFromKey(key)
		assert.Nil(t, err)
		assert.NotNil(t, signer)

		config := &ssh.ClientConfig{
			User:            vmConfig.Credientials.AdminUser,
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}

		addr := fmt.Sprintf("%s:22", vmConfig.PrimaryNetwork.PublicIP)
		conn, err := ssh.Dial("tcp", addr, config)
		assert.Nil(t, err)
		defer conn.Close()

		session, err := conn.NewSession()
		assert.Nil(t, err)
		session.Close()

		err = vmc.DeleteVM(ctx, vmConfig.Name)
		assert.Nil(t, err)
	}

}

func TestWindowsVMCreate(t *testing.T) {
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

	sshPublicKeySecretName := "VMSSHPublicKey"
	keyVault, err := NewKeyVault(ctx, vmc.Config.VaultURL, vmc.Config.AzureCredentials)
	assert.Nil(t, err)

	sshPublicKey, err := keyVault.GetSecret(ctx, sshPublicKeySecretName)
	assert.Nil(t, err)
	assert.NotNil(t, sshPublicKey)

	sshPrivateKeySecretName := "VMSSHPrivateKey"
	sshPrivateKey, err := keyVault.GetSecret(ctx, sshPrivateKeySecretName)
	assert.Nil(t, err)
	assert.NotNil(t, sshPrivateKey)

	/*
	   "imageReference": {
	   	"publisher": "MicrosoftWindowsDesktop",
	   	"offer": "Windows-10",
	   	"sku": "21h1-ent",
	   	"version": "latest"
	   },
	*/
	vmConfig := &cloudyvm.VirtualMachineConfiguration{
		ID:   "uvm-gotest",
		Name: "uvm-gotest",
		Size: &cloudyvm.VmSize{
			Size: "Standard_DS1_v2",
		},
		OSType:       "windows",
		Image:        "MicrosoftWindowsDesktop::Windows-10::21h1-ent",
		ImageVersion: "latest",
		Credientials: cloudyvm.Credientials{
			AdminUser:     "testadmin",
			AdminPassword: "TestPassword12#$",
			SSHKey:        sshPublicKey,
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
	err = vmc.CreateWindowsVirtualMachine(ctx, vmConfig)
	assert.Nil(t, err)

	if err == nil {
		block, _ := pem.Decode([]byte(sshPrivateKey))
		assert.NotNil(t, block)

		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		assert.Nil(t, err)

		signer, err := ssh.NewSignerFromKey(key)
		assert.Nil(t, err)
		assert.NotNil(t, signer)

		config := &ssh.ClientConfig{
			User:            vmConfig.Credientials.AdminUser,
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}

		addr := fmt.Sprintf("%s:22", vmConfig.PrimaryNetwork.PublicIP)
		conn, err := ssh.Dial("tcp", addr, config)
		assert.Nil(t, err)
		defer conn.Close()

		session, err := conn.NewSession()
		assert.Nil(t, err)
		session.Close()

		err = vmc.DeleteVM(ctx, vmConfig.Name)
		assert.Nil(t, err)
	}

}
