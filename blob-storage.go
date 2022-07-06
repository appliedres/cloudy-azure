package cloudyazure

import (
	"context"
	"errors"
	"fmt"
	"io"

	// "github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/appliedres/cloudy/storage"
)

// ObjectStorageManager  {
type BlobStorageAccount struct {
	Account    string
	AccountKey string
	UrlSlug    string
	Client     *azblob.ServiceClient
}

func NewBlobStorageAccount(ctx context.Context, account string, accountKey string, urlslug string) (*BlobStorageAccount, error) {
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
	return &BlobStorageAccount{
		Account:    account,
		AccountKey: accountKey,
		UrlSlug:    urlslug,
		Client:     service,
	}, err
}

func (sa *BlobStorageAccount) List(ctx context.Context) ([]*storage.StorageArea, error) {
	containers, err := sa.ListNative(ctx)
	if err != nil {
		return nil, err
	}

	rtn := make([]*storage.StorageArea, len(containers))
	for i, c := range containers {
		rtn[i] = sa.ToStorageArea(c)
	}
	return rtn, nil
}

func (sa *BlobStorageAccount) ToStorageArea(container *azblob.ContainerItem) *storage.StorageArea {
	return &storage.StorageArea{
		Name: *container.Name,
	}
}

func (sa *BlobStorageAccount) ListNative(ctx context.Context) ([]*azblob.ContainerItem, error) {
	var rtn []*azblob.ContainerItem

	pager := sa.Client.ListContainers(&azblob.ListContainersOptions{})
	for pager.NextPage(ctx) {
		if pager.Err() != nil {
			return nil, pager.Err()
		}
		items := pager.PageResponse().ContainerItems
		rtn = append(rtn, items...)
	}

	return rtn, nil
}

func (sa *BlobStorageAccount) Exists(ctx context.Context, key string) (bool, error) {
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
	containerClient, err := sa.Client.NewContainerClient(key)
	if err != nil {
		return nil, err
	}
	return NewBlobContainerFrom(ctx, containerClient), nil
}

func (sa *BlobStorageAccount) GetBlobContainer(ctx context.Context, name string) (*azblob.ContainerItem, error) {
	pager := sa.Client.ListContainers(&azblob.ListContainersOptions{
		Prefix: &name,
	})

	if pager.NextPage(ctx) {
		if pager.Err() != nil {
			return nil, pager.Err()
		}

		items := pager.PageResponse().ContainerItems
		if len(items) == 1 {
			return items[0], nil
		}
		if len(items) >= 1 {
			return items[0], nil
		}
	}

	return nil, nil
}

func (sa *BlobStorageAccount) Create(ctx context.Context, key string, openToPublic bool, tags map[string]string) (storage.ObjectStorage, error) {

	opts := &azblob.ContainerCreateOptions{
		Metadata: tags,
	}

	if openToPublic {
		opts.Access = azblob.PublicAccessTypeBlob.ToPtr()
	}

	_, err := sa.Client.CreateContainer(ctx, key, opts)
	if err != nil {
		return nil, err
	}

	// Created
	containerClient, err := sa.Client.NewContainerClient(key)
	if err != nil {
		return nil, err
	}
	return NewBlobContainerFrom(ctx, containerClient), nil
}

func (sa *BlobStorageAccount) Delete(ctx context.Context, key string) error {
	_, err := sa.Client.DeleteContainer(ctx, key, &azblob.ContainerDeleteOptions{})
	return err
}

// Object Storage
type BlobStorage struct {
	Account    string
	AccountKey string
	Container  string
	UrlSlug    string
	Client     *azblob.ContainerClient
}

func NewBlobContainerFrom(ctx context.Context, client *azblob.ContainerClient) *BlobStorage {
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

	c, err := bsa.Client.NewContainerClient(container)
	if err != nil {
		return nil, err
	}

	return &BlobStorage{
		Account:    account,
		AccountKey: accountKey,
		Container:  container,
		UrlSlug:    urlslug,
		Client:     c,
	}, err
}

func (b *BlobStorage) Upload(ctx context.Context, key string, data io.Reader, tags map[string]string) error {

	c, err := b.Client.NewBlockBlobClient(key)
	if err != nil {
		fmt.Printf("Error uploading, %v", err)
		return err
	}

	_, err = c.UploadStream(ctx, data, azblob.UploadStreamOptions{
		Metadata: tags,
	})

	return err
}

func (b *BlobStorage) Exists(ctx context.Context, key string) (bool, error) {
	var storageErr *azblob.StorageError

	c, err := b.Client.NewBlobClient(key)
	if err != nil {
		return false, err
	}

	_, err = c.GetProperties(ctx, nil)
	if err != nil && errors.As(err, &storageErr) {
		if storageErr.StatusCode() == 404 {
			return false, nil
		}
		return false, err
	}

	return true, nil

}

func (b *BlobStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	var storageErr *azblob.StorageError

	c, err := b.Client.NewBlockBlobClient(key)
	if err != nil {
		return nil, err
	}

	resp, err := c.Download(ctx, &azblob.BlobDownloadOptions{})
	if err != nil && errors.As(err, &storageErr) {
		return nil, err
	}

	return resp.Body(&azblob.RetryReaderOptions{}), err
}

func (b *BlobStorage) Delete(ctx context.Context, key string) error {
	c, err := b.Client.NewBlobClient(key)
	if err != nil {
		return err
	}
	_, err = c.Delete(ctx, &azblob.BlobDeleteOptions{})
	return err
}

func (b *BlobStorage) List(ctx context.Context, prefix string) ([]*storage.StoredObject, error) {
	items, _, err := b.ListNative(ctx, prefix)
	if err != nil {
		return nil, err
	}
	rtn := make([]*storage.StoredObject, len(items))
	for i, item := range items {
		rtn[i] = b.ToStoredObject(item)
	}

	return rtn, nil
}

func (b *BlobStorage) ToStoredObject(item *azblob.BlobItemInternal) *storage.StoredObject {
	return &storage.StoredObject{
		Key:  *item.Name,
		Tags: b.TagsToMap(item.BlobTags),
		Size: *item.Properties.ContentLength,
		MD5:  string(item.Properties.ContentMD5),
	}
}

func (b *BlobStorage) TagsToMap(tags *azblob.BlobTags) map[string]string {
	rtn := make(map[string]string)
	if tags != nil {
		for _, t := range tags.BlobTagSet {
			rtn[*t.Key] = *t.Value
		}
	}
	return rtn
}

func (b *BlobStorage) ListNative(ctx context.Context, prefix string) ([]*azblob.BlobItemInternal, []*azblob.BlobPrefix, error) {
	pager := b.Client.ListBlobsHierarchy("/", &azblob.ContainerListBlobsHierarchyOptions{
		Prefix: &prefix,
	})

	var items []*azblob.BlobItemInternal
	var prefixes []*azblob.BlobPrefix

	for pager.NextPage(ctx) {
		if pager.Err() != nil {
			return items, prefixes, pager.Err()
		}

		items = append(items, pager.PageResponse().Segment.BlobItems...)
		prefixes = append(prefixes, pager.PageResponse().Segment.BlobPrefixes...)
	}

	return items, prefixes, nil
}
