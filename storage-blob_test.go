package cloudyazure

import (
	"log"
	"testing"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/testutil"
)

func TestBlobAccount(t *testing.T) {
	ctx := cloudy.StartContext()
	testutil.LoadEnv("test.env")
	account := cloudy.ForceEnv("account", "")
	accountKey := cloudy.ForceEnv("accountKey", "")

	bsa, err := NewBlobStorageAccount(ctx, account, accountKey, "")
	if err != nil {
		log.Fatal(err)
		// t.FailNow()
	}

	testutil.TestObjectStorageManager(t, bsa)

}

func TestBlobFileAccount(t *testing.T) {
	ctx := cloudy.StartContext()
	testutil.LoadEnv("test.env")
	account := cloudy.ForceEnv("accountBlob", "")
	accountKey := cloudy.ForceEnv("accountKeyBlob", "")

	bfa, err := NewBlobContainerShare(ctx, account, accountKey, "")
	if err != nil {
		log.Fatal(err)
		// t.FailNow()
	}
	testutil.TestFileShareStorageManager(t, bfa, "file-storage-test")

}
