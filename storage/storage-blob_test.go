package storage

import (
	"log"
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestBlobAccount(t *testing.T) {
	testutil.MustSetTestEnv()
	env := cloudy.CreateEnvironment()

	ctx := cloudy.StartContext()
	// _ = testutil.LoadEnv("test.env")
	account := env.Force("account_blob", "")
	accountKey := env.Force("account_blob_Key", "")

	bsa, err := NewBlobStorageAccount(ctx, account, accountKey, "")
	if err != nil {
		log.Fatal(err)
		// t.FailNow()
	}

	testutil.TestObjectStorageManager(t, bsa)

}

func TestBlobFileAccount(t *testing.T) {
	ctx := cloudy.StartContext()
	// _ = testutil.LoadEnv("test.env")
	env := cloudy.CreateEnvironment()

	account := env.Force("accountBlob", "")
	accountKey := env.Force("accountBlobKey", "")

	bfa, err := NewBlobContainerShare(ctx, account, accountKey, "")
	if err != nil {
		log.Fatal(err)
		// t.FailNow()
	}
	// testutil.TestFileShareStorageManager(t, bfa, "file-storage-test")

	containerName := "adam-dyer"

	exists, err := bfa.Exists(ctx, containerName)
	assert.Nil(t, err)
	assert.False(t, exists)

	if !exists {
		tags := map[string]string{
			"Test": "Test",
		}

		_, err = bfa.Create(ctx, containerName, tags)
		assert.Nil(t, err)
	}

}
