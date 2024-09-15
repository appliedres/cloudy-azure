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
	Region         string
	TenantID       string
	ClientID       string
	ClientSecret   string
	ResourceGroup  string
	SubscriptionID string
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
	RegionPublic       = "public"
	RegionUSGovernment = "usgovernment"
)

func fixRegionName(regionName string) string {
	regionNameFixed := strings.ToLower(regionName)
	regionNameFixed = strings.ReplaceAll(regionNameFixed, "-", "")
	regionNameFixed = strings.ReplaceAll(regionNameFixed, "_", "")
	return regionNameFixed
}

func PolicyFromRegionString(regionName string) cloud.Configuration {
	regionNameFixed := fixRegionName(regionName)

	switch regionNameFixed {
	// Default to the government region
	case "":
		return cloud.AzureGovernment
	case RegionUSGovernment:
		return cloud.AzureGovernment
	case RegionPublic:
		return cloud.AzurePublic
	default:
		// Not sure WHAT to do with a custom.. Just assume it is the authority?
		// Needs to match "https://login.microsoftonline.com/"
		customRegion := regionName
		if !strings.HasPrefix(customRegion, "https://") {
			customRegion = fmt.Sprintf("https://%v", customRegion)
		}
		if !strings.HasSuffix(customRegion, "/") {
			customRegion = fmt.Sprintf("%v/", customRegion)
		}
		return cloud.Configuration{
			ActiveDirectoryAuthorityHost: customRegion,
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
		cloudRegion := PolicyFromRegionString(azcred.Region)
		creds, err := azidentity.NewClientSecretCredential(
			azcred.TenantID, azcred.ClientID, azcred.ClientSecret, &azidentity.ClientSecretCredentialOptions{
				ClientOptions: policy.ClientOptions{
					Cloud: cloudRegion,
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
		cloudRegion := PolicyFromRegionString(azcred.Region)
		creds, err := azidentity.NewEnvironmentCredential(&azidentity.EnvironmentCredentialOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloudRegion,
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
	}
	return credentials
}
