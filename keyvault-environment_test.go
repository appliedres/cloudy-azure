package cloudyazure

import (
	"context"
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func setUpKVE() (context.Context, *KeyVaultEnvironment, error) {
	em := testutil.CreateTestEnvMgr()
	ctx := cloudy.StartContext()

	vaultUrl := em.GetVar("AZ_VAULT_URL")
	creds := GetAzureCredentialsFromEnvMgr(cloudy.DefaultEnvManager)

	kve, err := NewKeyVaultEnvironmentService(ctx, vaultUrl, creds, "")

	return ctx, kve, err
}

func TestProvider(t *testing.T) {
	em := testutil.CreateTestEnvMgr()

	normal, err := cloudy.EnvironmentProviders.NewFromEnvMgrWith(em, KeyVaultId)
	assert.Nilf(t, err, "Error, %v", err)

	kveNormal := normal.(*KeyVaultEnvironment)
	assert.NotNil(t, kveNormal, "Should not be nil")

	cached, err := cloudy.EnvironmentProviders.NewFromEnvMgrWith(em, KeyVaultCachedId)

	assert.Nilf(t, err, "Error, %v", err)

	ce := cached.(*cloudy.CachedEnvironment)
	assert.NotNil(t, ce, "Should not be nil")

	kve := ce.Source.(*KeyVaultEnvironment)
	assert.NotNil(t, kve, "Should not be nil")
}

func TestSaveAndGet(t *testing.T) {
	all := make(map[string]string)

	all["TEST_KEY"] = "THIS IS JUST A TEST"

	ctx, kve, err := setUpKVE()
	if err != nil {
		t.Fatalf("Error creating key vault %v", err)
	}

	err = kve.SaveAll(ctx, all)
	assert.Nilf(t, err, "Error %v", err)

	v, err := kve.Get("TEST_KEY")
	assert.Nilf(t, err, "Error %v", err)

	assert.Equal(t, "THIS IS JUST A TEST", v)

	cachedKve := cloudy.NewTieredEnvironment(kve)
	v2, err := cachedKve.Get("TEST_KEY")
	assert.Nilf(t, err, "Error %v", err)
	assert.Equal(t, "THIS IS JUST A TEST", v2)

	v3, err := cachedKve.Get("TEST_KEY")
	assert.Nilf(t, err, "Error %v", err)
	assert.Equal(t, "THIS IS JUST A TEST", v3)

}

func TestSaveAndForce(t *testing.T) {
	all := make(map[string]string)

	all["TEST_KEY"] = "THIS IS JUST A TEST"

	ctx, kve, err := setUpKVE()
	if err != nil {
		t.Fatalf("Error creating key vault %v", err)
	}

	err = kve.SaveAll(ctx, all)
	assert.Nilf(t, err, "Error %v", err)

	v, err := kve.Get("TEST_KEY")
	assert.Nilf(t, err, "Error %v", err)

	assert.Equal(t, "THIS IS JUST A TEST", v)

	cachedKve := cloudy.NewTieredEnvironment(kve)
	v2, err := cachedKve.Force("TEST_KEY")
	assert.Nilf(t, err, "Error %v", err)
	assert.Equal(t, "THIS IS JUST A TEST", v2)

	v3, err := cachedKve.Get("TEST_KEY")
	assert.Nilf(t, err, "Error %v", err)
	assert.Equal(t, "THIS IS JUST A TEST", v3)

}
