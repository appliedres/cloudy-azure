package cloudyazure

import "github.com/appliedres/cloudy"

func init() {
	cloudy.CredentialSources[AzureCredentialsKey] = &AzureCredentialLoader{}
}

const AzureCredentialsKey = "azure"

type AzureCredentialLoader struct{}

func (loader *AzureCredentialLoader) ReadFromEnvMgr(em *cloudy.EnvManager) interface{} {

	return AzureCredentials{
		Region:       em.GetVar("AZ_REGION"),
		TenantID:     em.GetVar("AZ_TENANT_ID"),
		ClientID:     em.GetVar("AZ_CLIENT_ID"),
		ClientSecret: em.GetVar("AZ_CLIENT_SECRET"),
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
