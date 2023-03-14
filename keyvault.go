package cloudyazure

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/secrets"
)

const KeyVaultId = "azure-keyvault"
const KeyVaultCachedId = "azure-keyvault-cached"

func init() {
	secrets.SecretProviders.Register(KeyVaultId, &KeyVaultFactory{})
}

type KeyVaultFactory struct{}

type KeyVaultConfig struct {
	AzureCredentials
	VaultURL string `cloudyenv:"AZ_VAULT_URL"`
}

func (c *KeyVaultFactory) Create(cfg interface{}) (secrets.SecretProvider, error) {
	sec := cfg.(*KeyVaultConfig)
	if sec == nil {
		return nil, cloudy.ErrInvalidConfiguration
	}
	return NewKeyVault(context.Background(), sec.VaultURL, sec.AzureCredentials)
}

func (c *KeyVaultFactory) FromEnv(env *cloudy.Environment) (interface{}, error) {
	cfg := &KeyVaultConfig{}
	cfg.VaultURL = env.Force("AZ_VAULT_URL")
	cfg.AzureCredentials = GetAzureCredentialsFromEnv(env)
	return cfg, nil
}

type KeyVault struct {
	AzureCredentials
	VaultURL string
	Client   *azsecrets.Client
}

func NewKeyVault(ctx context.Context, vaultURL string, credentials AzureCredentials) (*KeyVault, error) {
	k := &KeyVault{
		AzureCredentials: credentials,
		VaultURL:         vaultURL,
	}
	err := k.Configure(ctx)
	return k, err
}

func (k *KeyVault) Configure(ctx context.Context) error {
	cred, err := GetAzureClientSecretCredential(k.AzureCredentials)
	if err != nil {
		return err
	}

	client, err := azsecrets.NewClient(k.VaultURL, cred, &azsecrets.ClientOptions{})
	if err != nil {
		return err
	}

	k.Client = client
	return nil
}

func (k *KeyVault) SaveSecretBinary(ctx context.Context, key string, secret []byte) error {
	key = sanitizeName(key)

	// Convert the binary to base64
	secretStr := base64.StdEncoding.EncodeToString(secret)
	return k.SaveSecret(ctx, key, secretStr)
}

func (k *KeyVault) GetSecretBinary(ctx context.Context, key string) ([]byte, error) {
	key = sanitizeName(key)

	secretStr, err := k.GetSecret(ctx, key)
	if err != nil {
		return nil, err
	}
	if secretStr == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(secretStr)
}

func (k *KeyVault) GetSecret(ctx context.Context, key string) (string, error) {
	key = sanitizeName(key)

	resp, err := k.Client.GetSecret(ctx, key, "", nil)

	if err != nil {
		if k.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}

	return *resp.Value, nil
}

// SaveSecret saves the secret in key vault. There are a few funny things that
// can happen here.
func (k *KeyVault) SaveSecret(ctx context.Context, key string, data string) error {
	key = sanitizeName(key)

	_, err := k.Client.SetSecret(ctx, key,
		azsecrets.SetSecretParameters{Value: &data}, nil)

	if k.IsConflictErr(err) {
		// A conflict means that we are trying to save a secret that has already been created
		// and then deleted, but it has not been purged.

		_, err = k.Client.RecoverDeletedSecret(ctx, key, &azsecrets.RecoverDeletedSecretOptions{})
		if err != nil {
			return err
		}

		// Need to sleep since Azure has an "eventually consistent" model... maybe try 3 time
		for i := -0; i < 3; i++ {
			time.Sleep(1 * time.Second)
			_, err = k.Client.SetSecret(ctx, key, azsecrets.SetSecretParameters{Value: &data}, nil)

			if err == nil {
				break
			}
		}
	}

	return err
}

func (k *KeyVault) IsConflictErr(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(strings.ToLower(err.Error()), "conflict")
}

func (k *KeyVault) DeleteSecret(ctx context.Context, key string) error {
	key = sanitizeName(key)

	_, err := k.Client.DeleteSecret(ctx, key, nil)

	if err != nil {
		// SEE : https://github.com/Azure/azure-sdk-for-go/issues/18321
		// This can happen if the keyvault does not have soft delete enabled.
		// This behavior is a bug and will be fixed by MS soon
		var respErr *azcore.ResponseError
		if !errors.As(err, &respErr) || respErr.StatusCode != 400 {
			fmt.Println(err.Error())
			return err
		}
		// nothing to do, got the expected error
	}

	return nil
}

func (k *KeyVault) IsNotFound(err error) bool {
	str := err.Error()
	return strings.Contains(str, "SecretNotFound")
}
