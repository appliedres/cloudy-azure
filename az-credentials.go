package cloudyazure

import "github.com/appliedres/cloudy"

func init() {
	cloudy.CredentialSources[AzureCredentialsKey] = &AzureCredentialLoader{}
}

const AzureCredentialsKey = "azure"

type AzureCredentialLoader struct{}

func (loader *AzureCredentialLoader) ReadFromEnv(env *cloudy.Environment) interface{} {
	region := env.Get("AZ_REGION")
	if region == "" {
		region = "usgovvirginia"
	}
	tenantId := env.Get("AZ_TENANT_ID")
	clientId := env.Get("AZ_CLIENT_ID")
	clientSecret := env.Get("AZ_CLIENT_SECRET")

	if tenantId == "" || clientId == "" || clientSecret == "" {
		return nil
	}

	return AzureCredentials{
		Region:       region,
		TenantID:     tenantId,
		ClientID:     clientId,
		ClientSecret: clientSecret,
	}

}
