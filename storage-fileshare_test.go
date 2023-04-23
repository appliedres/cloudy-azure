package cloudyazure

import (
	"log"
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestBlobFileshare(t *testing.T) {
	ctx := cloudy.StartContext()
	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")

	env := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "USER_API")
	vmCreds := env.LoadCredentials("VM_API")

	var factory AzureFileShareFactory
	b, err := factory.FromEnv(env.SegmentWithCreds(vmCreds, "TEST_FILE_SHARE"))
	bfa := b.(*BlobFileShare)

	if err != nil {
		log.Fatal(err)
	}

	shareName := "test-share"

	exists, err := bfa.Exists(ctx, shareName)
	assert.Nil(t, err)
	assert.False(t, exists)

	_, err = bfa.Create(ctx, shareName, nil)
	assert.Nil(t, err)

	exists, err = bfa.Exists(ctx, shareName)
	assert.Nil(t, err)
	assert.True(t, exists)

	err = bfa.Delete(ctx, shareName)
	assert.Nil(t, err)

	// testutil.TestFileShareStorageManager(t, bfa.(*BlobFileShare), "file-storage-test")

	// testutil.TestFileShareStorageManager(t, bfa.(*BlobFileShare), "Test-Share")
}
