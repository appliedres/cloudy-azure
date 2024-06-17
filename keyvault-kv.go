package cloudyazure

import (
	"context"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/keyvalue"
	"github.com/go-openapi/strfmt"
)

// Compile time assertions
var _ keyvalue.WritableSecureKeyValueStore = (*KeyVaultKvStore)(nil)

// var _ keyvalue.FilteredKeyValueStore = (*KeyVaultKvStore)(nil)

func init() {

}

type KeyVaultKvConfig struct {
	AzureCredentials
	VaultURL string
	Prefix   string
}

func NewAzureKeyVault(ctx context.Context, cfg *KeyVaultKvConfig) (*KeyVaultKvStore, error) {
	cloudy.Info(context.Background(), "KeyVaultKvStoreFactory Create")
	if cfg == nil {
		return nil, cloudy.ErrInvalidConfiguration
	}

	cloudy.Info(context.Background(), "KeyVault URL: %s", cfg.VaultURL)
	kve, err := NewKeyVaultKvStoreService(context.Background(), cfg.VaultURL, cfg.AzureCredentials, cfg.Prefix)
	if err != nil {
		return nil, cloudy.Error(context.Background(), "NewKeyVaultKvStoreService Error %v", err)
	}

	return kve, err
}

type KeyVaultKvStore struct {
	Vault  *KeyVault
	Prefix string
}

func NewKeyVaultKvStoreService(ctx context.Context, vaultURL string, credentials AzureCredentials, prefix string) (*KeyVaultKvStore, error) {
	k := &KeyVault{
		AzureCredentials: credentials,
		VaultURL:         vaultURL,
	}
	err := k.Configure(ctx)

	kve := &KeyVaultKvStore{
		Vault:  k,
		Prefix: prefix,
	}
	return kve, err
}

func (kve *KeyVaultKvStore) Get(key string) (string, error) {
	ctx := cloudy.StartContext()
	cloudy.Info(ctx, "KeyVaultKvStore Get: %s", key)

	val, err := kve.Vault.GetSecret(ctx, key)
	if err != nil {
		return "", cloudy.Error(ctx, "GetSecret (%s) error %v", key, err)
	}
	return val, nil
}

func (kve *KeyVaultKvStore) GetAll() (map[string]string, error) {
	ctx := context.Background()
	all, err := kve.Vault.GetAllSecrets(ctx)

	// Normalized Map
	rtn := make(map[string]string)
	for k, v := range all {
		rtn[cloudy.NormalizeKey(k)] = v
	}

	return rtn, err
}

func (kve *KeyVaultKvStore) SetMany(items map[string]string) error {
	ctx := context.Background()
	for k, v := range items {
		err := kve.Vault.SaveSecret(ctx, k, v)
		if err != nil {
			return err
		}
	}
	return nil
}

func (kve *KeyVaultKvStore) Delete(key string) error {
	return kve.Vault.DeleteSecret(context.Background(), key)
}

func (kve *KeyVaultKvStore) GetSecure(key string) (strfmt.Password, error) {
	val, err := kve.Get(key)
	if err != nil {
		return "", err
	}
	return strfmt.Password(val), nil
}

func (kve *KeyVaultKvStore) Set(key string, value string) error {
	ctx := context.Background()
	return kve.Vault.SaveSecret(ctx, key, value)
}

func (kve *KeyVaultKvStore) SetSecure(key string, value strfmt.Password) error {
	ctx := context.Background()
	return kve.Vault.SaveSecret(ctx, key, string(value))
}

// func (kve *KeyVaultKvStore) GetWithPrefix(prefix string) (map[string]string, error) {
// 	kve.Vault.
// 	// TODO:
// 	return nil, nil
// }
