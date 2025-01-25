package cloudyazure

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/appliedres/cloudy"
)

const DefaultRegion = "usgovvirginia"

type AzureCredentials struct {
	Type           string // Can be any type of CredType*
	Region         string // e.g. "usgovvirginia"
	TenantID       string
	ClientID       string
	ClientSecret   string
	ResourceGroup  string
	SubscriptionID string
	Cloud          string // e.g. azureusgovernment
}

const (
	CredTypeCli     = "cli"
	CredTypeDevCli  = "devcli"
	CredTypeSecret  = "secret"
	CredTypeCode    = "devicecode"
	CredTypeDefault = "default"
	CredTypeEnv     = "env"
	CredTypeBrowser = "browser"
	CredTypeManaged = "managed"
	CredTypeOther   = "other"
)

const (
	CloudPublic            = "public"
	CloudUSGovernment      = "usgovernment"
	CloudAzureUSGovernment = "azureusgovernment"
)

func FixCloudName(cloudName string) string {
	cloudNameFixed := strings.ToLower(cloudName)
	cloudNameFixed = strings.ReplaceAll(cloudNameFixed, "-", "")
	cloudNameFixed = strings.ReplaceAll(cloudNameFixed, "_", "")
	return cloudNameFixed
}

func PolicyFromCloudString(cloudName string) cloud.Configuration {
	cloudNameFixed := FixCloudName(cloudName)

	switch cloudNameFixed {
	// Default to the government cloud
	case "":
		return cloud.AzureGovernment
	case CloudUSGovernment:
		return cloud.AzureGovernment
	case CloudAzureUSGovernment:
		return cloud.AzureGovernment
	case CloudPublic:
		return cloud.AzurePublic
	default:
		// Not sure WHAT to do with a custom.. Just assume it is the authority?
		// Needs to match "https://login.microsoftonline.com/"
		customCloud := cloudName
		if !strings.HasPrefix(customCloud, "https://") {
			customCloud = fmt.Sprintf("https://%v", customCloud)
		}
		if !strings.HasSuffix(customCloud, "/") {
			customCloud = fmt.Sprintf("%v/", customCloud)
		}
		return cloud.Configuration{
			ActiveDirectoryAuthorityHost: customCloud,
			Services:                     map[cloud.ServiceName]cloud.ServiceConfiguration{},
		}
	}
}

func NewAzureCredentials(azcred *AzureCredentials) (azcore.TokenCredential, error) {
	// Determine the type of credentials. If the type is missing then we can guess
	// based on what has been provided.
	credType := strings.ToLower(azcred.Type)
	if credType == "" {
		if azcred.TenantID != "" && azcred.ClientID != "" && azcred.ClientSecret != "" {
			credType = CredTypeSecret
		} else {
			credType = CredTypeDefault
		}
	}

	switch credType {
	case CredTypeCli:
		opts := &azidentity.AzureCLICredentialOptions{}
		if azcred.TenantID != "" {
			opts.TenantID = azcred.TenantID
		}

		creds, err := azidentity.NewAzureCLICredential(opts)
		return creds, err

	case CredTypeDevCli:
		opts := &azidentity.AzureDeveloperCLICredentialOptions{}
		if azcred.TenantID != "" {
			opts.TenantID = azcred.TenantID
		}

		creds, err := azidentity.NewAzureDeveloperCLICredential(opts)
		return creds, err

	case CredTypeSecret:
		cloud := PolicyFromCloudString(azcred.Cloud)
		creds, err := azidentity.NewClientSecretCredential(
			azcred.TenantID, azcred.ClientID, azcred.ClientSecret, &azidentity.ClientSecretCredentialOptions{
				ClientOptions: policy.ClientOptions{
					Cloud: cloud,
				},
			},
		)
		return creds, err

	case CredTypeCode:
		creds, err := azidentity.NewDeviceCodeCredential(nil)
		return creds, err

	case CredTypeDefault:
		opts := &azidentity.DefaultAzureCredentialOptions{}
		if azcred.TenantID != "" {
			opts.TenantID = azcred.TenantID
		}

		creds, err := azidentity.NewDefaultAzureCredential(opts)
		return creds, err

	case CredTypeEnv:
		cloud := PolicyFromCloudString(azcred.Cloud)
		creds, err := azidentity.NewEnvironmentCredential(&azidentity.EnvironmentCredentialOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloud,
			},
		})
		return creds, err

	case CredTypeBrowser:
		creds, err := azidentity.NewInteractiveBrowserCredential(nil)
		return creds, err

	case CredTypeManaged:
		opts := &azidentity.ManagedIdentityCredentialOptions{}
		if azcred.ClientID != "" {
			opts.ID = azidentity.ClientID(azcred.ClientID)
		}

		creds, err := azidentity.NewManagedIdentityCredential(opts)
		return creds, err
	default:
		return nil, fmt.Errorf("unknown credential type, %v", credType)
	}

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
	credentials := AzureCredentials{
		Region:         env.Default("AZ_REGION", DefaultRegion),
		Type:           env.Default("AZ_CRED_TYPE", ""),
		TenantID:       env.Get("AZ_TENANT_ID"),
		ClientID:       env.Get("AZ_CLIENT_ID"),
		ClientSecret:   env.Get("AZ_CLIENT_SECRET"),
		ResourceGroup:  env.Get("AZ_RESOURCE_GROUP"),
		SubscriptionID: env.Get("AZ_SUBSCRIPTION_ID"),
		Cloud:          env.Get("AZ_CLOUD"),
	}
	return credentials
}
