package cloudyazure

import (
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
)

func TestBlobAccount(t *testing.T) {
	ctx := cloudy.StartContext()
	testutil.LoadEnv("test.env")
	account := cloudy.ForceEnv("account", "")
	accountKey := cloudy.ForceEnv("accountKey", "")

	bsa := NewBlobStorageAccount(ctx, account, accountKey, "")

	testutil.TestObjectStorageManager(t, bsa)
}
