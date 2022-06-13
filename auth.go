package cloudyazure

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

type AzureCredentials struct {
	TenantID     string
	ClientID     string
	ClientSecret string
	Region       string
}

func GetAzureCredentials(azCfg AzureCredentials) (*azidentity.ClientSecretCredential, error) {

	cred, err := azidentity.NewClientSecretCredential(azCfg.TenantID, azCfg.ClientID, azCfg.ClientSecret,
		&azidentity.ClientSecretCredentialOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloud.AzureGovernment,
			},
		})

	if err != nil {
		fmt.Printf("Error authentication provider: %v\n", err)
		return nil, err
	}

	return cred, err
}
