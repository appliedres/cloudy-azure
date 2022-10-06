package cloudyazure

import (
	"context"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/storage"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
)

var AzureFiles = "azure-files"

func init() {
	storage.FileShareProviders.Register(AzureFiles, &AzureFileShareFactory{})
}

type AzureFileShareFactory struct{}

func (f *AzureFileShareFactory) Create(cfg interface{}) (storage.FileStorageManager, error) {
	azCfg := cfg.(*BlobFileShare)
	if azCfg == nil {
		return nil, cloudy.ErrInvalidConfiguration
	}

	return NewBlobFileShare(context.Background(), azCfg)
}

func (f *AzureFileShareFactory) FromEnv(env *cloudy.SegmentedEnvironment) (interface{}, error) {
	cfg := &BlobFileShare{}
	cfg.Credentials = GetAzureCredentialsFromEnv(env)
	cfg.SubscriptionID = env.Force("AZ_SUBSCRIPTION_ID")
	cfg.ResourceGroupName = env.Force("AZ_RESOURCE_GROUP")
	cfg.SubscriptionID = env.Force("AZ_SUBSCRIPTION_ID")
	cfg.StorageAccountName = env.Force("AZ_ACCOUNT")

	return cfg, nil
}

// THe BlobFileShare provides file shares based on the Azure Blob Storage
type BlobFileShare struct {
	Client             *armstorage.FileSharesClient
	Credentials        AzureCredentials
	SubscriptionID     string
	ResourceGroupName  string
	StorageAccountName string
}

func NewBlobFileShare(ctx context.Context, cfg *BlobFileShare) (*BlobFileShare, error) {
	cred, err := GetAzureCredentials(cfg.Credentials)
	if err != nil {
		return nil, err
	}

	fileShareClient, err := armstorage.NewFileSharesClient(cfg.SubscriptionID,
		cred,
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
	key = sanitizeName(key)
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
	key = sanitizeName(key)
	_, err := bfs.Client.Get(ctx, bfs.ResourceGroupName,
		bfs.StorageAccountName, key, &armstorage.FileSharesClientGetOptions{})
	if err != nil {
		if is404(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
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

	key = sanitizeName(key)

	resp, err := bfs.Client.Create(ctx,
		bfs.ResourceGroupName,
		bfs.StorageAccountName,
		key,
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
	key = sanitizeName(key)
	_, err := bfs.Client.Delete(ctx, bfs.ResourceGroupName, bfs.StorageAccountName, key, &armstorage.FileSharesClientDeleteOptions{})
	return err
}
