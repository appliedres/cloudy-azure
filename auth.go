package cloudyazure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
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
		fmt.Printf("GetAzureCredentials Error authentication provider: %v\n", err)
		return nil, err
	}

	return cred, err
}

func GetAzureCredentialsFromEnvMgr(em *cloudy.EnvManager) AzureCredentials {
	cloudy.Info(context.Background(), "GetAzureCredentialsFromEnvMgr")

	// Check to see if there is already a set of credentials
	// TODO: re-enable creds?
	// creds := env.GetCredential(AzureCredentialsKey)
	// if creds != nil {
	// 	return creds.(AzureCredentials)
	// }

	return AzureCredentials{
		Region:       em.GetVar("AZ_REGION"),
		TenantID:     em.GetVar("AZ_TENANT_ID"),
		ClientID:     em.GetVar("AZ_CLIENT_ID"),
		ClientSecret: em.GetVar("AZ_CLIENT_SECRET"),
	}
}
