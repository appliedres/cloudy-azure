package cloudyazure

import (
	"context"
	"fmt"

	"github.com/appliedres/cloudy/storage"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

// THe BlobContainerShare provides file shares based on the Azure Blob Storage
type BlobContainerShare struct {
	Account    string
	AccountKey string
	UrlSlug    string
	Client     *azblob.ServiceClient
}

func NewBlobContainerShare(ctx context.Context, account string, accountKey string, urlslug string) (*BlobContainerShare, error) {
	if urlslug == "" {
		urlslug = "blob.core.usgovcloudapi.net"
	}

	cred, err := azblob.NewSharedKeyCredential(account, accountKey)
	if err != nil {
		return nil, err
	}

	service, err := azblob.NewServiceClientWithSharedKey(fmt.Sprintf("https://%s.%s/", account, urlslug), cred, nil)
	if err != nil {
		return nil, err
	}

	// handle(err)
	return &BlobContainerShare{
		Account:    account,
		AccountKey: accountKey,
		UrlSlug:    urlslug,
		Client:     service,
	}, err
}

func (bfs *BlobContainerShare) List(ctx context.Context) ([]*storage.FileShare, error) {
	pager := bfs.Client.ListContainers(&azblob.ListContainersOptions{})

	var rtn []*storage.FileShare

	for pager.NextPage(ctx) {
		if pager.Err() != nil {
			return nil, pager.Err()
		}
		for _, item := range pager.PageResponse().ContainerItems {
			rtn = append(rtn, &storage.FileShare{
				ID:   *item.Name,
				Name: *item.Name,
			})
		}
	}
	return rtn, nil
}

func (bfs *BlobContainerShare) Get(ctx context.Context, key string) (*storage.FileShare, error) {
	client, err := bfs.Client.NewContainerClient(key)
	if err != nil {
		return nil, err
	}

	_, err = client.GetProperties(ctx, &azblob.ContainerGetPropertiesOptions{})
	if err != nil {
		if is404(err) {
			return nil, nil
		}
		return nil, err
	}

	share := &storage.FileShare{
		ID:   key,
		Name: key,
	}

	return share, nil
}

func (bfs *BlobContainerShare) Exists(ctx context.Context, key string) (bool, error) {
	client, err := bfs.Client.NewContainerClient(key)
	if err != nil {
		return false, err
	}

	_, err = client.GetProperties(ctx, &azblob.ContainerGetPropertiesOptions{})
	if err != nil {
		if is404(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// func (bfs *BlobContainerShare) ContainerExists(ctx context.Context) (bool, error) {
// 	cred, err := GetAzureCredentials(bfs.Credentials)
// 	if err != nil {
// 		return false, err
// 	}

// 	blobContainerClient, err := armstorage.NewBlobContainersClient(bfs.SubscriptionID,
// 		cred,
// 		&arm.ClientOptions{
// 			ClientOptions: policy.ClientOptions{
// 				Cloud: cloud.AzureGovernment,
// 			},
// 		})
// 	if err != nil {
// 		return false, err
// 	}
// 	_, err = blobContainerClient.Get(ctx, bfs.ResourceGroupName, bfs.StorageAccountName, bfs.ContainerName, &armstorage.BlobContainersClientGetOptions{})
// 	if err != nil {
// 		if is404(err) {
// 			return false, nil
// 		}
// 		return false, err
// 	}
// 	return true, nil
// }

// func (bfs *BlobContainerShare) ContainerCreate(ctx context.Context) error {
// 	cred, err := GetAzureCredentials(bfs.Credentials)
// 	if err != nil {
// 		return err
// 	}

// 	serviceUrl := fmt.Sprintf("https://%s.%s/", bfs.StorageAccountName, "blob.core.usgovcloudapi.net")
// 	client, err := azblob.NewServiceClient(serviceUrl,cred, &azblob.ClientOptions{} )
// 	if err != nil {
// 		return err
// 	}
// 	if err != nil {
// 		return err
// 	}

// 	metadata := map[string]string{
// 		"Drive_name":        "Personal",
// 		"Drive_owner":       bfs.UPN,
// 		"Drive_description": "Your personal drive. Keep personal files here",
// 	}

// 	client.CreateContainer(ctx, bfs.ContainerName, &azblob.ContainerCreateOptions{
// 		Metadata: metadata,
// 	})

// 	_, err = client.Create(ctx,
// 		bfs.ResourceGroupName,
// 		bfs.StorageAccountName,
// 		bfs.ContainerName,
// 		armstorage.BlobContainer{
// 			ContainerProperties: &armstorage.ContainerProperties{
// 				PublicAccess:          to.Ptr(armstorage.PublicAccessNone),
// 				EnableNfsV3RootSquash: to.Ptr(false),
// 				Metadata:              metadata,
// 			},
// 		},
// 		nil)

// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }

// func (bfs *BlobContainerShare) ContainerCreate2(ctx context.Context) error {
// 	cred, err := GetAzureCredentials(bfs.Credentials)
// 	if err != nil {
// 		return err
// 	}

// 	blobContainerClient, err := armstorage.NewBlobContainersClient(bfs.SubscriptionID,
// 		cred,
// 		&arm.ClientOptions{
// 			ClientOptions: policy.ClientOptions{
// 				Cloud: cloud.AzureGovernment,
// 			},
// 		})
// 	if err != nil {
// 		return err
// 	}

// 	metadata := map[string]*string{
// 		"Drive_name":        to.Ptr("Personal"),
// 		"Drive_owner":       to.Ptr(bfs.UPN),
// 		"Drive_description": to.Ptr("Your personal drive. Keep personal files here"),
// 	}

// 	_, err = blobContainerClient.Create(ctx,
// 		bfs.ResourceGroupName,
// 		bfs.StorageAccountName,
// 		bfs.ContainerName,
// 		armstorage.BlobContainer{
// 			ContainerProperties: &armstorage.ContainerProperties{
// 				PublicAccess:          to.Ptr(armstorage.PublicAccessNone),
// 				EnableNfsV3RootSquash: to.Ptr(false),
// 				Metadata:              metadata,
// 			},
// 		},
// 		nil)

// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }

func (bfs *BlobContainerShare) Create(ctx context.Context, key string, tags map[string]string) (*storage.FileShare, error) {
	// Create the file share if necessary
	// az storage share-rm create
	// 	-g $AZ_APP_RESOURCE_GROUP
	// 	--storage-account $AZ_HOME_DIRS_STORAGE_ACCOUNT
	// 	--name $UPN_short_lower
	// 	--enabled-protocol NFS
	// 	--root-squash NoRootSquash
	// 	--quota 100
	client, err := bfs.Client.NewContainerClient(key)
	if err != nil {
		return nil, err
	}

	_, err = client.Create(ctx, &azblob.ContainerCreateOptions{
		Metadata: tags,
	})
	if err != nil {
		return nil, err
	}

	share := &storage.FileShare{
		ID:   key,
		Name: key,
	}

	return share, nil
}

func (bfs *BlobContainerShare) Delete(ctx context.Context, key string) error {
	client, err := bfs.Client.NewContainerClient(key)
	if err != nil {
		return err
	}
	_, err = client.Delete(ctx, &azblob.ContainerDeleteOptions{})
	return err
}
