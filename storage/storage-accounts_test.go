package storage

import (
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestStorageAccount(t *testing.T) {
	testutil.MustSetTestEnv()
	env := cloudy.CreateEnvironment()

	ctx := cloudy.StartContext()
	// _ = testutil.LoadEnv("test.env")
	account := env.Force("accountBlob", "")
	// accountKey := env.Force("accountBlobKey", "")

	accountType, err := GetStorageAccountType(ctx, cloudy.DefaultEnvironment, account)
	assert.Nil(t, err)
	cloudy.Info(ctx, "%s", accountType)
	assert.NotNil(t, accountType)

}
