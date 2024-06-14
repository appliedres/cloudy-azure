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
	var requiredEnvDefs = []cloudy.EnvDefinition{
		{
			Key:		  "HOME_FILE_SHARE_AZ_RESOURCE_GROUP",
			Name:         "HOME_FILE_SHARE_AZ_RESOURCE_GROUP",
			FallbackKeys: []string{"AZ_RESOURCE_GROUP"},
		}, {
			Key:		  "HOME_FILE_SHARE_AZ_ACCOUNT",
			Name:         "HOME_FILE_SHARE_AZ_ACCOUNT",
		}, {
			Key:		  "HOME_FILE_SHARE_AZ_SUBSCRIPTION_ID",
			Name:         "HOME_FILE_SHARE_AZ_SUBSCRIPTION_ID",
			FallbackKeys: []string{"AZ_SUBSCRIPTION_ID"},
		},
	}

	storage.FileShareProviders.Register(AzureFiles, &AzureFileShareFactory{}, requiredEnvDefs)
}

type AzureFileShareFactory struct{}

func (f *AzureFileShareFactory) Create(cfg interface{}) (storage.FileStorageManager, error) {
	bfs := cfg.(*BlobFileShare)
	if bfs == nil {
		return nil, cloudy.ErrInvalidConfiguration
	}

	err := bfs.Connect(context.Background())
	if err != nil {
		return nil, err
	}

	return bfs, nil
}

func (f *AzureFileShareFactory) FromEnvMgr(em *cloudy.EnvManager, prefix string) (interface{}, error) {
	bfs := &BlobFileShare{}
	bfs.Credentials = GetAzureCredentialsFromEnvMgr(em)
	bfs.ResourceGroupName = em.GetVar(prefix+"_AZ_RESOURCE_GROUP", "AZ_RESOURCE_GROUP")
	bfs.StorageAccountName = em.GetVar(prefix+"_AZ_ACCOUNT", "AZ_ACCOUNT")
	bfs.SubscriptionID = em.GetVar(prefix+"_AZ_SUBSCRIPTION_ID", "AZ_SUBSCRIPTION_ID")

	err := bfs.Connect(context.Background())
	if err != nil {
		return nil, err
	}

	return bfs, nil
}

// THe BlobFileShare provides file shares based on the Azure Blob Storage
type BlobFileShare struct {
	Client             *armstorage.FileSharesClient
	Credentials        AzureCredentials
	SubscriptionID     string
	ResourceGroupName  string
	StorageAccountName string
}

func (bfs *BlobFileShare) Connect(ctx context.Context) error {
	cred, err := GetAzureClientSecretCredential(bfs.Credentials)
	if err != nil {
		return err
	}

	fileShareClient, err := armstorage.NewFileSharesClient(bfs.SubscriptionID,
		cred,
		&arm.ClientOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloud.AzureGovernment,
			},
		})

	if err != nil {
		return err
	}

	bfs.Client = fileShareClient
	return nil
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
	cloudy.Info(ctx, "BlobFileShare.Exists: %s", key)
	key = sanitizeName(key)
	cloudy.Info(ctx, "BlobFileShare.Exists (sanitized): %s", key)

	cloudy.Info(ctx, "BlobFileShare.Exists.Client.Get: %s, %s, %s", bfs.ResourceGroupName, bfs.StorageAccountName, key)
	_, err := bfs.Client.Get(ctx, bfs.ResourceGroupName,
		bfs.StorageAccountName, key, &armstorage.FileSharesClientGetOptions{})
	if err != nil {
		if is404(err) {
			cloudy.Info(ctx, "BlobFileShare 404 Does not Exist: %s", key)
			return false, nil
		}
		_ = cloudy.Error(ctx, "BlobFileShare.Exists: %s, %v", key, err)
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

	cloudy.Info(ctx, "BlobFileShare.Create: %s", key)

	key = sanitizeName(key)

	cloudy.Info(ctx, "BlobFileShare.Create.Client.Create: %s, %s, %s", bfs.ResourceGroupName, bfs.StorageAccountName, key)
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

		_ = cloudy.Error(ctx, "BlobFileShare.Create: %s, %v", key, err)
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
