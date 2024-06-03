package cloudyazure

import (
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/secrets"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestKeyVault(t *testing.T) {
	em := testutil.CreateTestEnvMgr()
	ctx := cloudy.StartContext()

	kv, err := NewKeyVaultFromEnvMgr(em)
	assert.Nil(t, err)

	if err == nil {
		secrets.SecretsTest(t, ctx, kv)
	}
}
