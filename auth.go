package cloudyazure

import (
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

func GetAzureCredentialsFromEnv(env *cloudy.Environment) AzureCredentials {
	// Check to see if there is already a set of credentials
	creds := env.GetCredential(AzureCredentialsKey)
	if creds != nil {
		return creds.(AzureCredentials)
	}

	region := env.Get("AZ_REGION")
	if region == "" {
		region = DefaultRegion
	}
	return AzureCredentials{
		Region:       region,
		TenantID:     env.Force("AZ_TENANT_ID"),
		ClientID:     env.Force("AZ_CLIENT_ID"),
		ClientSecret: env.Force("AZ_CLIENT_SECRET"),
	}
}

func GetKeyVaultAzureCredentialsFromEnv(env *cloudy.Environment) AzureCredentials {
	region := env.Get("KEYVAULT_AZ_REGION")
	if region == "" {
		region = DefaultRegion
	}

	return AzureCredentials{
		Region:       region,
		TenantID:     env.Force("KEYVAULT_AZ_TENANT_ID"),
		ClientID:     env.Force("KEYVAULT_AZ_CLIENT_ID"),
		ClientSecret: env.Force("KEYVAULT_AZ_CLIENT_SECRET"),
	}
}
