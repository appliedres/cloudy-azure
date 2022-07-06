package cloudyazure

import (
	"context"

	"github.com/appliedres/cloudy/storage"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
)

// THe BlobFileShare provides file shares based on the Azure Blob Storage
type BlobFileShare struct {
	Client             *armstorage.FileSharesClient
	Credentials        *azidentity.ClientSecretCredential
	SubscriptionID     string
	ResourceGroupName  string
	StorageAccountName string
	ContainerName      string
	UPN                string
}

func NewBlobFileShare(ctx context.Context, cfg *BlobFileShare) (*BlobFileShare, error) {
	fileShareClient, err := armstorage.NewFileSharesClient(cfg.SubscriptionID,
		cfg.Credentials,
		&arm.ClientOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloud.AzureGovernment,
			},
		})
	if err != nil {
		return nil, err
	}

	cfg.Client = fileShareClient
	return cfg, nil
}

func (bfs *BlobFileShare) List(ctx context.Context) ([]*storage.FileShare, error) {
	pager := bfs.Client.NewListPager(bfs.ResourceGroupName, bfs.StorageAccountName, &armstorage.FileSharesClientListOptions{})

	var rtn []*storage.FileShare

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return rtn, err
		}
		for _, item := range resp.Value {
			rtn = append(rtn, &storage.FileShare{
				ID:   *item.ID,
				Name: *item.Name,
			})
		}
	}
	return rtn, nil
}

func (bfs *BlobFileShare) Get(ctx context.Context, key string) (*storage.FileShare, error) {
	resp, err := bfs.Client.Get(ctx, bfs.ResourceGroupName, bfs.StorageAccountName, key, &armstorage.FileSharesClientGetOptions{})
	if err != nil {
		return nil, err
	}
	share := &storage.FileShare{
		ID:   *resp.ID,
		Name: *resp.Name,
	}

	return share, nil
}

func (bfs *BlobFileShare) Exists(ctx context.Context, key string) (bool, error) {
	// Check If the File share already exists
	// az storage share-rm exists
	// 	-g $AZ_APP_RESOURCE_GROUP
	// 	--storage-account $AZ_HOME_DIRS_STORAGE_ACCOUNT
	// 	--name $UPN_short_lower

	_, err := bfs.Client.Get(ctx, bfs.ResourceGroupName, bfs.StorageAccountName, key, &armstorage.FileSharesClientGetOptions{})
	if err != nil {
		return false, err
	}

	return true, nil
}

func (bfs *BlobFileShare) ContainerExists(ctx context.Context) (bool, error) {
	blobContainerClient, err := armstorage.NewBlobContainersClient(bfs.SubscriptionID,
		bfs.Credentials,
		&arm.ClientOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloud.AzureGovernment,
			},
		})
	if err != nil {
		return false, err
	}
	_, err = blobContainerClient.Get(ctx, bfs.ResourceGroupName, bfs.StorageAccountName, bfs.ContainerName, &armstorage.BlobContainersClientGetOptions{})
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (bfs *BlobFileShare) ContainerCreate(ctx context.Context) error {
	blobContainerClient, err := armstorage.NewBlobContainersClient(bfs.SubscriptionID,
		bfs.Credentials,
		&arm.ClientOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloud.AzureGovernment,
			},
		})
	if err != nil {
		return err
	}

	metadata := map[string]*string{
		"Drive_name":        to.Ptr("Personal"),
		"Drive_owner":       to.Ptr(bfs.UPN),
		"Drive_description": to.Ptr("Your personal drive. Keep personal files here"),
	}

	_, err = blobContainerClient.Create(ctx,
		bfs.ResourceGroupName,
		bfs.StorageAccountName,
		bfs.ContainerName,
		armstorage.BlobContainer{
			ContainerProperties: &armstorage.ContainerProperties{
				PublicAccess:          to.Ptr(armstorage.PublicAccessNone),
				EnableNfsV3RootSquash: to.Ptr(false),
				Metadata:              metadata,
			},
		},
		nil)

	if err != nil {
		return err
	}

	return nil
}

func (bfs *BlobFileShare) Create(ctx context.Context, key string, tags map[string]string) (*storage.FileShare, error) {
	cExists, err := bfs.ContainerExists(ctx)
	if err != nil {
		return nil, err
	}
	if !cExists {
		err = bfs.ContainerCreate(ctx)
		if err != nil {
			return nil, err
		}
	}

	// Create the file share if necessary
	// az storage share-rm create
	// 	-g $AZ_APP_RESOURCE_GROUP
	// 	--storage-account $AZ_HOME_DIRS_STORAGE_ACCOUNT
	// 	--name $UPN_short_lower
	// 	--enabled-protocol NFS
	// 	--root-squash NoRootSquash
	// 	--quota 100

	resp, err := bfs.Client.Create(ctx,
		bfs.ResourceGroupName,
		bfs.StorageAccountName,
		bfs.ContainerName,
		armstorage.FileShare{
			FileShareProperties: &armstorage.FileShareProperties{
				ShareQuota:       to.Ptr(int32(100)),
				RootSquash:       to.Ptr(armstorage.RootSquashTypeNoRootSquash),
				EnabledProtocols: to.Ptr(armstorage.EnabledProtocolsNFS),
			}},
		&armstorage.FileSharesClientCreateOptions{
			Expand: nil,
		},
	)
	if err != nil {
		return nil, err
	}

	share := &storage.FileShare{
		ID:   *resp.ID,
		Name: *resp.Name,
	}

	return share, nil
}

func (bfs *BlobFileShare) Delete(ctx context.Context, key string) error {
	_, err := bfs.Client.Delete(ctx, bfs.ResourceGroupName, bfs.StorageAccountName, key, &armstorage.FileSharesClientDeleteOptions{})
	return err
}
