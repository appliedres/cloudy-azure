package cloudyazure

import (
	"context"

	"github.com/appliedres/cloudy/storage"
)

// THe BlobFileShare provides file shares based on the Azure Blob Storage
type BlobFileShare struct {
}

func (bfs *BlobFileShare) List(ctx context.Context) ([]*storage.FileShare, error) {

	return nil, nil
}

func (bfs *BlobFileShare) Get(ctx context.Context, key string) (*storage.FileShare, error) {

	return nil, nil
}

func (bfs *BlobFileShare) Exists(ctx context.Context, key string) (bool, error) {
	// Check If the File share already exists
	// az storage share-rm exists
	// 	-g $AZ_APP_RESOURCE_GROUP
	// 	--storage-account $AZ_HOME_DIRS_STORAGE_ACCOUNT
	// 	--name $UPN_short_lower

	return false, nil
}

func (bfs *BlobFileShare) Create(ctx context.Context, key string, tags map[string]string) (*storage.FileShare, error) {

	// Create the file share if necessary
	// az storage share-rm create
	// 	-g $AZ_APP_RESOURCE_GROUP
	// 	--storage-account $AZ_HOME_DIRS_STORAGE_ACCOUNT
	// 	--name $UPN_short_lower
	// 	--enabled-protocol NFS
	// 	--root-squash NoRootSquash
	// 	--quota 100
	return nil, nil
}

func (bfs *BlobFileShare) Delete(ctx context.Context, key string) error {

	return nil
}
