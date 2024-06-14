package cloudyazure

import (
	"context"
	"log"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	"github.com/appliedres/cloudy"
)

const DefaultRegion = "usgovvirginia"

type AzureCredentials struct {
	TenantID     string
	ClientID     string
	ClientSecret string
	Region       string
}

func GetAzureClientSecretCredential(azCfg AzureCredentials) (*azidentity.ClientSecretCredential, error) {

	cred, err := azidentity.NewClientSecretCredential(azCfg.TenantID, azCfg.ClientID, azCfg.ClientSecret,
		&azidentity.ClientSecretCredentialOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloud.AzureGovernment,
			},
		})

	if err != nil {
		log.Fatalf("GetAzureCredentials Error authentication provider: %v\n", err)
		return nil, err
	}

	return cred, err
}

func GetAzureCredentialsFromEnvMgr(em *cloudy.EnvManager) AzureCredentials {
	cloudy.Info(context.Background(), "GetAzureCredentialsFromEnvMgr")

	return AzureCredentials{
		Region:       em.GetVar("AZ_REGION"),
		TenantID:     em.GetVar("AZ_TENANT_ID"),
		ClientID:     em.GetVar("AZ_CLIENT_ID"),
		ClientSecret: em.GetVar("AZ_CLIENT_SECRET"),
	}
}

func GetNSGIdByName(subscriptionID, resourceGroupName, nsgName string) (string, error) {
	
	cred, err := GetAzureClientSecretCredential(GetAzureCredentialsFromEnvMgr(cloudy.DefaultEnvManager))
	if err != nil {
		log.Fatal(err)
	}

	options := arm.ClientOptions {
		ClientOptions: azcore.ClientOptions {
			Cloud: cloud.AzureGovernment,
		},
	}

	networkClientFactory, err := armnetwork.NewClientFactory(subscriptionID, cred, &options)
	if err != nil {
		log.Fatal(err)
	}
	securityGroupsClient := networkClientFactory.NewSecurityGroupsClient()

	nsg, err := securityGroupsClient.Get(context.Background(), resourceGroupName, nsgName, &armnetwork.SecurityGroupsClientGetOptions{})
	if err != nil {
		log.Fatal(err)
	}

	// Return the NSG ID
	return *nsg.ID, nil
}