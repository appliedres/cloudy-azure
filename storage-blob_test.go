package cloudyazure

import (
	"log"
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestBlobAccount(t *testing.T) {
	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")
	env := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(env)

	ctx := cloudy.StartContext()
	_ = testutil.LoadEnv("test.env")
	account := env.Force("ACCOUNT_BLOB")
	accountKey := env.Force("ACCOUNT_BLOB_KEY")

	bsa, err := NewBlobStorageAccount(ctx, account, accountKey, "")
	if err != nil {
		log.Fatal(err)
		// t.FailNow()
	}

	testutil.TestObjectStorageManager(t, bsa)

}

func TestBlobFileAccount(t *testing.T) {
	_ = testutil.LoadEnv("../arkloud-conf/arkloud.env")
	env := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "USERAPI_PREFIX", "KEYVAULT")
	cloudy.SetDefaultEnvironment(env)

	ctx := cloudy.StartContext()
	_ = testutil.LoadEnv("test.env")
	account := env.Force("ACCOUNT_BLOB")
	accountKey := env.Force("ACCOUNT_BLOB_KEY")

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
