package cloudyazure

import (
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestStorageAccount(t *testing.T) {
	testutil.MustSetTestEnv()
	envMgr := cloudy.NewEnvManager("TestEnv")

	ctx := cloudy.StartContext()
	// _ = testutil.LoadEnv("test.env")
	account := envMgr.GetVar("accountBlob")
	// accountKey := em.GetVar("accountBlobKey", "")

	accountType, err := GetStorageAccountType(ctx, envMgr, account)
	assert.Nil(t, err)
	cloudy.Info(ctx, "%s", accountType)
	assert.NotNil(t, accountType)

}
