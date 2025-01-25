package keyvault

import (
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/secrets"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestKeyVault(t *testing.T) {
	env := testutil.CreateTestEnvironment()
	ctx := cloudy.StartContext()

	kv, err := NewKeyVaultFromEnv(env)
	assert.Nil(t, err)

	if err == nil {
		secrets.SecretsTest(t, ctx, kv)
	}
}
