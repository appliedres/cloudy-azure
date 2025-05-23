package storage

import (
	"context"
	"strings"

	"github.com/appliedres/cloudy"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy/storage"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

var AzureBlob = "azure-blob"

func init() {
	storage.FileShareProviders.Register(AzureBlob, &AzureBlobFileShareFactory{})
}

type AzureBlobFileShareFactory struct{}

func (f *AzureBlobFileShareFactory) Create(cfg interface{}) (storage.FileStorageManager, error) {
	azCfg := cfg.(*AzureStorageAccount)
	if azCfg == nil {
		return nil, cloudy.ErrInvalidConfiguration
	}

	cloudy.Info(context.Background(), "NewBlobContainerShare account: %s", azCfg.AccountName)

	return NewBlobContainerShare2(context.Background(), azCfg)
}

func (f *AzureBlobFileShareFactory) FromEnv(env *cloudy.Environment) (interface{}, error) {
	cfg := &AzureStorageAccount{}
	cfg.AccountName = env.Force("AZ_ACCOUNT")
	cfg.AccountKey = env.Force("AZ_ACCOUNT_KEY")
	return cfg, nil
}

// THe BlobContainerShare provides file shares based on the Azure Blob Storage
type BlobContainerShare struct {
	Account string
	// AccountKey string
	// UrlSlug    string
	Client *azblob.Client
}

func NewBlobContainerShare2(ctx context.Context, acct *AzureStorageAccount) (*BlobContainerShare, error) {
	serviceUrl := acctStorageUrl(acct)
	var client *azblob.Client
	if acct.AccountKey != "" {
		cred, err := azblob.NewSharedKeyCredential(acct.AccountName, acct.AccountKey)
		if err != nil {
			return nil, err
		}

		client, err = azblob.NewClientWithSharedKeyCredential(serviceUrl, cred, nil)
		if err != nil {
			return nil, err
		}

	} else {
		cred, err := cloudyazure.NewAzureCredentials(&acct.AzureCredentials)
		if err != nil {
			return nil, err
		}
		client, err = azblob.NewClient(serviceUrl, cred, nil)
		if err != nil {
			return nil, err
		}
	}

	return &BlobContainerShare{
		Account: acct.AccountName,
		// AccountKey: acct,
		// UrlSlug:    urlslug,
		Client: client,
	}, nil
}

func NewBlobContainerShare(ctx context.Context, account string, accountKey string, urlslug string) (*BlobContainerShare, error) {
	cfg := &AzureStorageAccount{
		AccountName: account,
		AccountKey:  accountKey,
		UrlSlug:     urlslug,
	}
	return NewBlobContainerShare2(ctx, cfg)
}

func (bfs *BlobContainerShare) List(ctx context.Context) ([]*storage.FileShare, error) {
	pager := bfs.Client.NewListContainersPager(&azblob.ListContainersOptions{})

	var rtn []*storage.FileShare

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range resp.ContainerItems {
			rtn = append(rtn, &storage.FileShare{
				ID:   *item.Name,
				Name: *item.Name,
			})
		}
	}
	return rtn, nil
}

func (bfs *BlobContainerShare) Get(ctx context.Context, key string) (*storage.FileShare, error) {
	client := bfs.Client.ServiceClient().NewContainerClient(key)

	_, err := client.GetProperties(ctx, &container.GetPropertiesOptions{})
	if err != nil {
		if cloudyazure.Is404(err) {
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
	cloudy.Info(ctx, "BlobContainerShare.Exists in %s: %s", bfs.Account, key)
	key = cloudyazure.SanitizeName(key)
	cloudy.Info(ctx, "BlobContainerShare.Exists (sanitized): %s", key)

	pager := bfs.Client.NewListContainersPager(&azblob.ListContainersOptions{
		Include: azblob.ListContainersInclude{
			Metadata: true,  // Include Metadata
			Deleted:  false, // Include deleted containers in the result as well
		},
	})

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return false, err
		}

		for _, container := range resp.ContainerItems {
			if strings.EqualFold(*container.Name, key) {
				return true, nil
			}
		}
	}

	return false, nil
}

func (bfs *BlobContainerShare) Create(ctx context.Context, key string, tags map[string]string) (*storage.FileShare, error) {
	// Create the file share if necessary
	// az storage share-rm create
	// 	-g $AZ_APP_RESOURCE_GROUP
	// 	--storage-account $AZ_HOME_DIRS_STORAGE_ACCOUNT
	// 	--name $UPN_short_lower
	// 	--enabled-protocol NFS
	// 	--root-squash NoRootSquash
	// 	--quota 100
	key = cloudyazure.SanitizeName(key)
	client := bfs.Client.ServiceClient().NewContainerClient(key)

	_, err := client.Create(ctx, &container.CreateOptions{
		Metadata: cloudyazure.ToStrPointerMap(tags),
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
	key = cloudyazure.SanitizeName(key)
	client := bfs.Client.ServiceClient().NewContainerClient(key)

	_, err := client.Delete(ctx, &container.DeleteOptions{})
	return err
}
