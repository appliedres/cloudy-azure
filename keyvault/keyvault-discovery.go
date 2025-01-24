package keyvault

// import (
// 	"context"
// 	"fmt"
// 	"strings"

// 	"github.com/appliedres/cloudy"
// 	"github.com/appliedres/cloudy/secrets"

// 	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
// 	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
// 	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
// 	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
// )

// const KeyVaultDiscoveryId = "azure-keyvault"

// func init() {
// 	secrets.SecretDiscoveryProviders.Register(KeyVaultDiscoveryId, &KeyVaultDiscoveryFactory{})
// }

// // --- FACTORY ----

// type KeyVaultDiscoveryFactory struct{}

// type KeyVaultDiscoveryConfig struct {
// 	AzureCredentials
// 	SubscriptionID string
// }

// func (c *KeyVaultDiscoveryFactory) Create(cfg interface{}) (secrets.SecretDiscoveryProvider, error) {
// 	sec := cfg.(*KeyVaultDiscoveryConfig)
// 	if sec == nil {
// 		return nil, cloudy.ErrInvalidConfiguration
// 	}
// 	return NewKeyVaultDiscovery(context.Background(), sec.SubscriptionID, sec.AzureCredentials)
// }

// func (c *KeyVaultDiscoveryFactory) FromEnv(env *cloudy.Environment) (interface{}, error) {
// 	cfg := &KeyVaultConfig{}
// 	cfg.AzureCredentials = GetAzureCredentialsFromEnv(env)
// 	return cfg, nil
// }

// func (c *KeyVaultDiscoveryFactory) ListRequiredEnv(env *cloudy.Environment) []string {
// 	cred := AzureGetRequiredEnv()
// 	return cred
// }

// // --- Provider ----
// type KeyVaultDiscovery struct {
// 	AzureCredentials
// 	SubscriptionID string
// 	Client         *armkeyvault.VaultsClient
// }

// func NewKeyVaultDiscovery(ctx context.Context, subscriptionId string, credentials AzureCredentials) (*KeyVaultDiscovery, error) {
// 	k := &KeyVaultDiscovery{
// 		AzureCredentials: credentials,
// 		SubscriptionID:   subscriptionId,
// 	}
// 	err := k.Configure(ctx)
// 	return k, err
// }

// func NewKeyVaultDiscoveryFromEnv(env *cloudy.Environment) (*KeyVaultDiscovery, error) {
// 	cfg := &KeyVaultDiscovery{}
// 	cfg.AzureCredentials = GetAzureCredentialsFromEnv(env)
// 	cfg.SubscriptionID = env.Force("AZ_SUBSCRIPTION_ID")
// 	return NewKeyVaultDiscovery(context.Background(), cfg.SubscriptionID, cfg.AzureCredentials)
// }

// func (k *KeyVaultDiscovery) Configure(ctx context.Context) error {
// 	cred, err := GetAzureClientSecretCredential(k.AzureCredentials)
// 	if err != nil {
// 		return err
// 	}

// 	client, err := armkeyvault.NewVaultsClient(k.SubscriptionID, cred, &arm.ClientOptions{
// 		ClientOptions: policy.ClientOptions{
// 			Cloud: cloud.AzureGovernment,
// 		},
// 	})

// 	if err != nil {
// 		return err
// 	}

// 	k.Client = client
// 	return nil
// }

// func (k *KeyVaultDiscovery) ListAll(ctx context.Context) ([]*secrets.SecretStoreDescription, error) {
// 	var rtn []*secrets.SecretStoreDescription
// 	pager := k.Client.NewListPager(&armkeyvault.VaultsClientListOptions{})
// 	for {
// 		page, err := pager.NextPage(ctx)
// 		if err != nil {
// 			return rtn, err
// 		}

// 		for _, item := range page.Value {
// 			// Tag from description
// 			tags := make(map[string]string)

// 			for k, v := range item.Tags {
// 				fmt.Printf("\t%v : %v\n", k, v)
// 				tags[k] = *v
// 			}

// 			// example ID
// 			// /subscriptions/fc11c55b-59e8-4714-87db-404319407761/resourceGroups/arkloud-753db1545cdb9829-arkloud-rg-hub/providers/Microsoft.KeyVault/vaults/arkloudhubjumpboxkv7exjo,
// 			id := *item.ID
// 			parts := strings.Split(id, "/")
// 			resourceGroup := parts[4]

// 			vault, err := k.Client.Get(ctx, resourceGroup, *item.Name, &armkeyvault.VaultsClientGetOptions{})
// 			if err != nil {
// 				return rtn, err
// 			}

// 			rtn = append(rtn, &secrets.SecretStoreDescription{
// 				ID:          *item.ID,
// 				Name:        *item.Name,
// 				Description: "",
// 				Tags:        tags,
// 				URI:         *vault.Properties.VaultURI,
// 			})

// 		}

// 		if page.NextLink == nil {
// 			break
// 		}
// 	}

// 	return rtn, nil
// }
