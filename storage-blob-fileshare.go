package cloudyazure

import (
	"context"
	"fmt"
	"strings"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/storage"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

var AzureBlob = "azure-blob"

func init() {
	var requiredEnvDefs = []cloudy.EnvDefinition{
		{
			Key:		  "PERSONAL_FILE_SHARE_AZ_ACCOUNT",
			Name:         "PERSONAL_FILE_SHARE_AZ_ACCOUNT",			
		}, {
			Key:		  "PERSONAL_FILE_SHARE_AZ_ACCOUNT_KEY",
			Name:         "PERSONAL_FILE_SHARE_AZ_ACCOUNT_KEY",
		}, {
			Key:		  "GROUP_FILE_SHARE_AZ_ACCOUNT",
			Name:         "GROUP_FILE_SHARE_AZ_ACCOUNT",
		}, {
			Key:		  "GROUP_FILE_SHARE_AZ_ACCOUNT_KEY",
			Name:         "GROUP_FILE_SHARE_AZ_ACCOUNT_KEY",
		}, {
			Key:		  "HOME_FILE_SHARE_AZ_ACCOUNT",
			Name:         "HOME_FILE_SHARE_AZ_ACCOUNT",
		},
	}

	storage.FileShareProviders.Register(AzureBlob, &AzureBlobFileShareFactory{}, requiredEnvDefs)
}

type AzureBlobFileShareFactory struct{}

func (f *AzureBlobFileShareFactory) Create(cfg interface{}) (storage.FileStorageManager, error) {
	azCfg := cfg.(*BlobContainerShare)
	if azCfg == nil {
		return nil, cloudy.ErrInvalidConfiguration
	}

	cloudy.Info(context.Background(), "NewBlobContainerShare account: %s", azCfg.Account)

	return NewBlobContainerShare(context.Background(), azCfg.Account, azCfg.AccountKey, azCfg.UrlSlug)
}

func (f *AzureBlobFileShareFactory) FromEnvMgr(em *cloudy.EnvManager, prefix string) (interface{}, error) {
	cfg := &BlobContainerShare{}
	cfg.Account = em.GetVar(prefix+"_AZ_ACCOUNT", "AZ_ACCOUNT")
	cfg.AccountKey = em.GetVar(prefix+"_AZ_ACCOUNT_KEY", "AZ_ACCOUNT_KEY")

	return cfg, nil
}

// THe BlobContainerShare provides file shares based on the Azure Blob Storage
type BlobContainerShare struct {
	Account    string
	AccountKey string
	UrlSlug    string
	Client     *azblob.Client
}

func NewBlobContainerShare(ctx context.Context, account string, accountKey string, urlslug string) (*BlobContainerShare, error) {
	if urlslug == "" {
		urlslug = "blob.core.usgovcloudapi.net"
	}

	cred, err := azblob.NewSharedKeyCredential(account, accountKey)
	if err != nil {
		return nil, err
	}

	serviceUrl := fmt.Sprintf("https://%s.%s/", account, urlslug)
	cloudy.Info(ctx, "NewBlobContainerShare %s", serviceUrl)

	service, err := azblob.NewClientWithSharedKeyCredential(serviceUrl, cred, nil)
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
	cloudy.Info(ctx, "BlobContainerShare.Exists in %s: %s", bfs.Account, key)
	key = sanitizeName(key)
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
	key = sanitizeName(key)
	client := bfs.Client.ServiceClient().NewContainerClient(key)

	_, err := client.Create(ctx, &container.CreateOptions{
		Metadata: ToStrPointerMap(tags),
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
	key = sanitizeName(key)
	client := bfs.Client.ServiceClient().NewContainerClient(key)

	_, err := client.Delete(ctx, &container.DeleteOptions{})
	return err
}
