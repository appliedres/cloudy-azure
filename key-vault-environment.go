package cloudyazure

import (
	"context"

	"github.com/appliedres/cloudy"
)

func init() {
	cloudy.EnvironmentProviders.Register(KeyVaultId, &KeyVaultEnvironmentFactory{})
	cloudy.EnvironmentProviders.Register(KeyVaultCachedId, &KeyVaultEnvironmentCachedFactory{})
}

type KeyVaultEnvironmentConfig struct {
	AzureCredentials
	VaultURL string
	Prefix   string
}

type KeyVaultEnvironmentFactory struct{}

func (c *KeyVaultEnvironmentFactory) Create(cfg interface{}) (cloudy.EnvironmentService, error) {
	sec := cfg.(*KeyVaultEnvironmentConfig)
	if sec == nil {
		return nil, cloudy.ErrInvalidConfiguration
	}
	kve, err := NewKeyVaultEnvironmentService(context.Background(), sec.VaultURL, sec.AzureCredentials, sec.Prefix)
	return kve, err
}

func (c *KeyVaultEnvironmentFactory) FromEnv(env *cloudy.Environment) (interface{}, error) {
	cfg := &KeyVaultEnvironmentConfig{}
	cfg.VaultURL = env.Force("AZ_VAULT_URL")
	cfg.AzureCredentials = GetAzureCredentialsFromEnv(env)
	cfg.Prefix = env.Get("prefix")

	return cfg, nil
}

type KeyVaultEnvironmentCachedFactory struct{}

func (c *KeyVaultEnvironmentCachedFactory) Create(cfg interface{}) (cloudy.EnvironmentService, error) {
	sec := cfg.(*KeyVaultEnvironmentConfig)
	if sec == nil {
		return nil, cloudy.ErrInvalidConfiguration
	}
	kve, err := NewKeyVaultEnvironmentService(context.Background(), sec.VaultURL, sec.AzureCredentials, sec.Prefix)
	if err != nil {
		return nil, err
	}
	return cloudy.NewCachedEnvironment(kve), nil
}

func (c *KeyVaultEnvironmentCachedFactory) FromEnv(env *cloudy.Environment) (interface{}, error) {
	cfg := &KeyVaultEnvironmentConfig{}
	cfg.VaultURL = env.Force("AZ_VAULT_URL")
	cfg.AzureCredentials = GetAzureCredentialsFromEnv(env)
	cfg.Prefix = env.Get("prefix")

	return cfg, nil
}

type KeyVaultEnvironment struct {
	Vault  *KeyVault
	Prefix string
}

func NewKeyVaultEnvironmentService(ctx context.Context, vaultURL string, credentials AzureCredentials, prefix string) (*KeyVaultEnvironment, error) {
	k := &KeyVault{
		AzureCredentials: credentials,
		VaultURL:         vaultURL,
	}
	err := k.Configure(ctx)

	kve := &KeyVaultEnvironment{
		Vault:  k,
		Prefix: prefix,
	}
	return kve, err
}

func LoadEnvironment(ctx context.Context) (*cloudy.Environment, error) {
	return nil, nil
}

func (kve *KeyVaultEnvironment) Get(name string) (string, error) {
	ctx := cloudy.StartContext()

	val, err := kve.Vault.GetSecret(ctx, name)
	if err != nil {
		return "", err
	}
	if val == "" {
		return "", cloudy.ErrKeyNotFound
	}
	return val, nil
}

func (kve *KeyVaultEnvironment) SaveAll(ctx context.Context, items map[string]string) error {
	for k, v := range items {
		name := cloudy.NormalizeEnvName(k)
		err := kve.Vault.SaveSecret(ctx, name, v)
		if err != nil {
			return err
		}
	}
	return nil
}
