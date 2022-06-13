package cloudyazure

import (
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
)

func TestBlobAccount(t *testing.T) {
	ctx := cloudy.StartContext()

	account := "publictest"
	accountKey := "/D13RjfAT3d/p45rM94SgF48BwQEwTIPuXxn9tMa958bJLRGB2Oa1ApHmqTvRTY7tQH+vyP60WbF+AStkqA4RQ=="

	bsa := NewBlobStorageAccount(ctx, account, accountKey, "")

	testutil.TestObjectStorageManager(t, bsa)
}
