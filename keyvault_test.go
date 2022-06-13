package cloudyazure

import (
	"context"
	"testing"

	"github.com/appliedres/cloudy/secrets"
	"github.com/stretchr/testify/assert"
)

func TestKeyVault(t *testing.T) {

	creds := AzureCredentials{
		TenantID:     "848c7ae1-3864-4c5c-8ec0-dc90b8e05ade",
		ClientID:     "84bed11f-3739-404b-879d-5d27d7648d60",
		ClientSecret: "6U_.4Lh-jW0ofss~fzcGf-hRmYlcZl25kY",
	}
	vaultURL := "https://gokeyvault.vault.usgovcloudapi.net/"

	kv, err := NewKeyVault(context.Background(), vaultURL, creds)
	assert.Nil(t, err)

	secrets.SecretsTest(t, context.Background(), kv)
}

func TestKeyVaultDynamic(t *testing.T) {

}
