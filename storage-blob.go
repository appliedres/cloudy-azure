package cloudyazure

import (
	"context"
	"fmt"
	"io"

	// "github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/storage"
)

func init() {
	storage.ObjectStorageProviders.Register(AzureBlob, &AzureBlobStorageFactory{})
}

type AzureBlobStorageFactory struct{}

func (f *AzureBlobStorageFactory) Create(cfg interface{}) (storage.ObjectStorageManager, error) {
	azCfg := cfg.(*BlobContainerShare)
	if azCfg == nil {
		return nil, cloudy.ErrInvalidConfiguration
	}

	return NewBlobStorageAccount(context.Background(), azCfg.Account, azCfg.AccountKey, azCfg.UrlSlug)
}

func (f *AzureBlobStorageFactory) FromEnv(env *cloudy.Environment) (interface{}, error) {
	cfg := &BlobContainerShare{}
	cfg.Account = env.Force("AZ_ACCOUNT")
	cfg.AccountKey = env.Force("AZ_ACCOUNT_KEY")

	return cfg, nil
}

// ObjectStorageManager  {
type BlobStorageAccount struct {
	Account    string
	AccountKey string
	UrlSlug    string
	Client     *azblob.Client
}

func NewBlobStorageAccount(ctx context.Context, account string, accountKey string, urlslug string) (*BlobStorageAccount, error) {
	if urlslug == "" {
		urlslug = "blob.core.usgovcloudapi.net"
	}
	serviceUrl := fmt.Sprintf("https://%s.%s/", account, urlslug)

	cred, err := azblob.NewSharedKeyCredential(account, accountKey)
	if err != nil {
		return nil, err
	}

	service, err := azblob.NewClientWithSharedKeyCredential(serviceUrl, cred, nil)
	if err != nil {
		return nil, err
	}

	// handle(err)
	return &BlobStorageAccount{
		Account:    account,
		AccountKey: accountKey,
		UrlSlug:    urlslug,
		Client:     service,
	}, err
}

func (sa *BlobStorageAccount) List(ctx context.Context) ([]*storage.StorageArea, error) {

	rtn := []*storage.StorageArea{}

	pager := sa.Client.NewListContainersPager(&azblob.ListContainersOptions{
		Include: azblob.ListContainersInclude{
			Metadata: true,
		},
	})
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, containerItem := range resp.ContainerItems {
			rtn = append(rtn, &storage.StorageArea{
				Name: *containerItem.Name,
				Tags: FromStrPointerMap(containerItem.Metadata),
			})
		}
	}

	return rtn, nil
}

func (sa *BlobStorageAccount) Exists(ctx context.Context, key string) (bool, error) {
	cloudy.Info(ctx, "BlobStorageAccount.Exists: %s", key)
	c, err := sa.GetBlobContainer(ctx, key)
	if err != nil {
		return false, err
	}
	if c == nil {
		return false, nil
	}
	return true, nil
}

func (sa *BlobStorageAccount) Get(ctx context.Context, key string) (storage.ObjectStorage, error) {
	key = sanitizeName(key)

	containerClient := sa.Client.ServiceClient().NewContainerClient(key)

	return NewBlobContainerFrom(ctx, containerClient), nil
}

func (sa *BlobStorageAccount) GetItem(ctx context.Context, key string) (*storage.StorageArea, error) {
	key = sanitizeName(key)

	cloudy.Info(ctx, "BlobStorageAccount.GetItem: %s", key)

	storageArea, err := sa.GetBlobContainer(ctx, key)

	if err != nil {
		return nil, err
	}

	return storageArea, nil
}

func (sa *BlobStorageAccount) GetBlobContainer(ctx context.Context, name string) (*storage.StorageArea, error) {
	pager := sa.Client.NewListContainersPager(&azblob.ListContainersOptions{
		Prefix: &name,
		Include: azblob.ListContainersInclude{
			Metadata: true,
		},
	})

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		items := resp.ContainerItems

		if len(items) >= 1 {
			if len(items) > 1 {
				cloudy.Warn(ctx, "GetBlobContainer found more than 1 result, returning first result only")
			}

			return &storage.StorageArea{
				Name: *items[0].Name,
				Tags: FromStrPointerMap(items[0].Metadata),
			}, nil

		}
	}

	return nil, nil
}

func (sa *BlobStorageAccount) Create(ctx context.Context, key string, openToPublic bool, tags map[string]string) (storage.ObjectStorage, error) {

	opts := &azblob.CreateContainerOptions{
		Metadata: ToStrPointerMap(tags),
	}

	if openToPublic {
		opts.Access = to.Ptr(azblob.PublicAccessTypeBlob)
	}

	_, err := sa.Client.CreateContainer(ctx, key, opts)
	if err != nil {
		return nil, err
	}

	// Created
	key = sanitizeName(key)
	containerClient := sa.Client.ServiceClient().NewContainerClient(key)

	return NewBlobContainerFrom(ctx, containerClient), nil
}

func (sa *BlobStorageAccount) Delete(ctx context.Context, key string) error {
	_, err := sa.Client.DeleteContainer(ctx, key, &azblob.DeleteContainerOptions{})
	return err
}

// Object Storage
type BlobStorage struct {
	Account    string
	AccountKey string
	Container  string
	UrlSlug    string
	Client     *container.Client
}

func NewBlobContainerFrom(ctx context.Context, client *container.Client) *BlobStorage {
	return &BlobStorage{
		Client: client,
	}
}

func NewBlobContainer(ctx context.Context, account string, accountKey string, container string, urlslug string) (*BlobStorage, error) {
	if urlslug == "" {
		urlslug = "blob.core.usgovcloudapi.net"
	}

	bsa, err := NewBlobStorageAccount(ctx, account, accountKey, urlslug)
	if err != nil {
		return nil, err
	}

	container = sanitizeName(container)
	c := bsa.Client.ServiceClient().NewContainerClient(container)

	return &BlobStorage{
		Account:    account,
		AccountKey: accountKey,
		Container:  container,
		UrlSlug:    urlslug,
		Client:     c,
	}, err
}

func (b *BlobStorage) Upload(ctx context.Context, key string, data io.Reader, tags map[string]string) error {

	c := b.Client.NewBlockBlobClient(key)

	_, err := c.UploadStream(ctx, data, &blockblob.UploadStreamOptions{
		Metadata: ToStrPointerMap(tags),
	})

	return err
}

func (b *BlobStorage) Exists(ctx context.Context, key string) (bool, error) {
	cloudy.Info(ctx, "BlobStorage.Exists: %s", key)

	// c := b.Client.NewBlobClient(key)

	_, err := b.Client.GetProperties(ctx, nil)
	if err != nil {
		if bloberror.HasCode(err, bloberror.ResourceNotFound) {
			return false, nil
		}
		return false, err
	}

	return true, nil

}

func (b *BlobStorage) UpdateMetadata(ctx context.Context, key string, tags map[string]string) error {
	exists, err := b.Exists(ctx, "")
	if err != nil {
		return err
	}
	if !exists {
		return cloudy.Error(ctx, "Cannot update metadata, %s doesn't exist", key)
	}

	metadata := map[string]*string{}

	for k, v := range tags {
		metadata[k] = &v
	}

	_, err = b.Client.SetMetadata(ctx, &container.SetMetadataOptions{
		Metadata: metadata,
	})
	if err != nil {
		return cloudy.Error(ctx, "Error updating metadata: %v", err)
	}

	return nil
}

func (b *BlobStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {

	c := b.Client.NewBlockBlobClient(key)

	resp, err := c.DownloadStream(ctx, &azblob.DownloadStreamOptions{})
	if err != nil {
		return nil, err
	}

	return resp.Body, err
}

func (b *BlobStorage) Delete(ctx context.Context, key string) error {
	c := b.Client.NewBlobClient(key)

	_, err := c.Delete(ctx, &azblob.DeleteBlobOptions{})
	return err
}

func (b *BlobStorage) List(ctx context.Context, prefix string) ([]*storage.StoredObject, []*storage.StoredPrefix, error) {
	items, prefixes, err := b.ListNative(ctx, prefix)
	if err != nil {
		return nil, nil, err
	}
	rtn := make([]*storage.StoredObject, len(items))
	for i, item := range items {
		rtn[i] = b.ToStoredObject(item)
	}

	rtnPrefixes := make([]*storage.StoredPrefix, len(prefixes))
	for i, item := range prefixes {
		rtnPrefixes[i] = &storage.StoredPrefix{
			Key: *item.Name,
		}
	}

	return rtn, rtnPrefixes, err
}

func (b *BlobStorage) ToStoredObject(item *container.BlobItem) *storage.StoredObject {
	return &storage.StoredObject{
		Key:  *item.Name,
		Tags: b.TagsToMap(item.BlobTags),
		Size: *item.Properties.ContentLength,
		MD5:  string(item.Properties.ContentMD5),
	}
}

func (b *BlobStorage) TagsToMap(tags *container.BlobTags) map[string]string {
	rtn := make(map[string]string)
	if tags != nil {
		for _, t := range tags.BlobTagSet {
			rtn[*t.Key] = *t.Value
		}
	}
	return rtn
}

func (b *BlobStorage) ListNative(ctx context.Context, prefix string) ([]*container.BlobItem, []*container.BlobPrefix, error) {

	pager := b.Client.NewListBlobsHierarchyPager("/", &container.ListBlobsHierarchyOptions{
		Prefix: &prefix,
	})

	var items []*container.BlobItem
	var prefixes []*container.BlobPrefix

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return items, prefixes, err
		}

		items = append(items, resp.Segment.BlobItems...)
		prefixes = append(prefixes, resp.Segment.BlobPrefixes...)
	}

	return items, prefixes, nil
}
