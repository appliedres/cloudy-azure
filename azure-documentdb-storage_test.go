package cloudyazure

import (
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

var (
	// bad DB connection config
	badDbConnection = AzureCosmosDbDatastore{
		url: "https://badurl.com",
		key: "food",
	}
)

func TestOpen(t *testing.T) {

	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")

	newEnv := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(newEnv)
	ctx := cloudy.StartContext()

	vaultUrl := newEnv.Force("AZ_VAULT_URL")
	creds := GetAzureCredentialsFromEnv(cloudy.DefaultEnvironment)

	kve, _ := NewKeyVaultEnvironmentService(ctx, vaultUrl, creds, "")

	_ = cloudy.NewTieredEnvironment(
		cloudy.NewTestFileEnvironmentService(),
		kve,
	)

	ctx = cloudy.StartContext()

	var a interface{}
	assert.Nil(t, badDbConnection.Open(ctx, a))
}

func TestClose(t *testing.T) {

	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")

	newEnv := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(newEnv)
	ctx := cloudy.StartContext()

	vaultUrl := newEnv.Force("AZ_VAULT_URL")
	creds := GetAzureCredentialsFromEnv(cloudy.DefaultEnvironment)

	kve, _ := NewKeyVaultEnvironmentService(ctx, vaultUrl, creds, "")

	_ = cloudy.NewTieredEnvironment(
		cloudy.NewTestFileEnvironmentService(),
		kve,
	)

	ctx = cloudy.StartContext()

	assert.Nil(t, badDbConnection.Close(ctx))
}

func TestSave(t *testing.T) {

	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")

	newEnv := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(newEnv)
	ctx := cloudy.StartContext()

	vaultUrl := newEnv.Force("AZ_VAULT_URL")
	creds := GetAzureCredentialsFromEnv(cloudy.DefaultEnvironment)

	kve, _ := NewKeyVaultEnvironmentService(ctx, vaultUrl, creds, "")

	_ = cloudy.NewTieredEnvironment(
		cloudy.NewTestFileEnvironmentService(),
		kve,
	)

	ctx = cloudy.StartContext()

	var a interface{}
	key := "key"
	assert.NotNil(t, badDbConnection.Save(ctx, a, key))
}

func TestGet(t *testing.T) {

	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")

	newEnv := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(newEnv)
	ctx := cloudy.StartContext()

	vaultUrl := newEnv.Force("AZ_VAULT_URL")
	creds := GetAzureCredentialsFromEnv(cloudy.DefaultEnvironment)

	kve, _ := NewKeyVaultEnvironmentService(ctx, vaultUrl, creds, "")

	_ = cloudy.NewTieredEnvironment(
		cloudy.NewTestFileEnvironmentService(),
		kve,
	)

	ctx = cloudy.StartContext()

	badKey := "key"
	rtn, error := badDbConnection.Get(ctx, badKey)
	assert.Nil(t, rtn)
	assert.NotNil(t, error)
}

func TestGetAll(t *testing.T) {

	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")

	newEnv := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(newEnv)
	ctx := cloudy.StartContext()

	vaultUrl := newEnv.Force("AZ_VAULT_URL")
	creds := GetAzureCredentialsFromEnv(cloudy.DefaultEnvironment)

	kve, _ := NewKeyVaultEnvironmentService(ctx, vaultUrl, creds, "")

	_ = cloudy.NewTieredEnvironment(
		cloudy.NewTestFileEnvironmentService(),
		kve,
	)

	ctx = cloudy.StartContext()

	rtn, error := badDbConnection.GetAll(ctx)
	assert.Nil(t, rtn)
	assert.NotNil(t, error)
}

func TestExists(t *testing.T) {

	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")

	newEnv := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(newEnv)
	ctx := cloudy.StartContext()

	vaultUrl := newEnv.Force("AZ_VAULT_URL")
	creds := GetAzureCredentialsFromEnv(cloudy.DefaultEnvironment)

	kve, _ := NewKeyVaultEnvironmentService(ctx, vaultUrl, creds, "")

	_ = cloudy.NewTieredEnvironment(
		cloudy.NewTestFileEnvironmentService(),
		kve,
	)

	ctx = cloudy.StartContext()

	badKey := "key"
	rtn, error := badDbConnection.Get(ctx, badKey)
	assert.Nil(t, rtn)
	assert.NotNil(t, error)
}

func TestDelete(t *testing.T) {

	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")

	newEnv := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(newEnv)
	ctx := cloudy.StartContext()

	vaultUrl := newEnv.Force("AZ_VAULT_URL")
	creds := GetAzureCredentialsFromEnv(cloudy.DefaultEnvironment)

	kve, _ := NewKeyVaultEnvironmentService(ctx, vaultUrl, creds, "")

	_ = cloudy.NewTieredEnvironment(
		cloudy.NewTestFileEnvironmentService(),
		kve,
	)

	ctx = cloudy.StartContext()

	badKey := "key"
	error := badDbConnection.Delete(ctx, badKey)
	assert.NotNil(t, error)
}

func TestPing(t *testing.T) {

	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")

	newEnv := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(newEnv)
	ctx := cloudy.StartContext()

	vaultUrl := newEnv.Force("AZ_VAULT_URL")
	creds := GetAzureCredentialsFromEnv(cloudy.DefaultEnvironment)

	kve, _ := NewKeyVaultEnvironmentService(ctx, vaultUrl, creds, "")

	_ = cloudy.NewTieredEnvironment(
		cloudy.NewTestFileEnvironmentService(),
		kve,
	)

	ctx = cloudy.StartContext()

	rtn := badDbConnection.Ping(ctx)
	assert.False(t, rtn)
}
