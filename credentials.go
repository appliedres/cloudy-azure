package cloudyazure

import "github.com/appliedres/cloudy"

func init() {
	cloudy.CredentialSources[AzureCredentialsKey] = &AzureCredentialLoader{}
}

const AzureCredentialsKey = "azure"

type AzureCredentialLoader struct{}

func (loader *AzureCredentialLoader) ReadFromEnv(env *cloudy.Environment) interface{} {

	return AzureCredentials{
		Region:       env.Default("AZ_REGION", DefaultRegion),
		TenantID:     env.Force("AZ_TENANT_ID"),
		ClientID:     env.Force("AZ_CLIENT_ID"),
		ClientSecret: env.Force("AZ_CLIENT_SECRET"),
	}

}

func AzureGetRequiredEnv() []string {
	return []string{
		"AZ_REGION",
		"AZ_TENANT_ID",
		"AZ_CLIENT_ID",
		"AZ_CLIENT_SECRET",
	}
}
