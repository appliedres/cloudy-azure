package cloudyazure

import (
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/appliedres/cloudy/storage"
)

type BlobStorageAccount struct {
	Account    string
	AccountKey string
	UrlSlug    string
}

func NewBlobStorageAccount(ctx context.Context, account string, accountKey string, urlslug string) *BlobStorageAccount {
	if urlslug == "" {
		urlslug = "blob.core.usgovcloudapi.net"
	}

	return &BlobStorageAccount{
		Account:    account,
		AccountKey: accountKey,
		UrlSlug:    urlslug,
	}
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
		Name: container.Name,
	}
}

func (sa *BlobStorageAccount) ListNative(ctx context.Context) ([]*azblob.ContainerItem, error) {
	Url, err := sa.ServiceUrl()
	if err != nil {
		return nil, err
	}

	marker := azblob.Marker{}
	options := azblob.ListContainersSegmentOptions{}

	resp, err := Url.ListContainersSegment(ctx, marker, options)
	if err != nil {
		return nil, err
	}

	rtn := make([]*azblob.ContainerItem, len(resp.ContainerItems))
	for i := range resp.ContainerItems {
		rtn[i] = &resp.ContainerItems[i]
	}
	return rtn, nil
}

func (sa *BlobStorageAccount) Get(ctx context.Context, key string) (storage.ObjectStorage, error) {
	return NewBlobContainer(ctx, sa.Account, sa.AccountKey, key, sa.UrlSlug), nil
}

func (sa *BlobStorageAccount) GetBlobContainer(ctx context.Context, name string) (*azblob.ContainerItem, error) {
	Url, err := sa.ServiceUrl()
	if err != nil {
		return nil, err
	}

	marker := azblob.Marker{}
	options := azblob.ListContainersSegmentOptions{
		Prefix: name,
	}

	resp, err := Url.ListContainersSegment(ctx, marker, options)
	if err != nil {
		return nil, err
	}

	for _, c := range resp.ContainerItems {
		if c.Name == name {
			return &c, nil
		}
	}

	return nil, nil
}

func (sa *BlobStorageAccount) Create(ctx context.Context, key string, openToPublic bool, tags map[string]string) (storage.ObjectStorage, error) {
	svc, err := sa.ServiceUrl()
	if err != nil {
		return nil, err
	}

	var pat azblob.PublicAccessType
	if openToPublic {
		pat = azblob.PublicAccessBlob
	} else {
		pat = azblob.PublicAccessNone
	}
	meta := azblob.Metadata(tags)

	containerSvc := svc.NewContainerURL(key)
	_, err = containerSvc.Create(ctx, meta, pat)
	if err != nil {
		return nil, err
	}

	// Created
	return NewBlobContainer(ctx, sa.Account, sa.AccountKey, key, sa.UrlSlug), nil
}

func (sa *BlobStorageAccount) Delete(ctx context.Context, key string) error {
	cSvc, err := sa.ContainerUrl(key)
	if err != nil {
		return err
	}
	var conds azblob.ContainerAccessConditions
	_, err = cSvc.Delete(ctx, conds)
	return err
}

func (sa *BlobStorageAccount) ServiceUrl() (*azblob.ServiceURL, error) {
	urlslug := sa.UrlSlug
	if urlslug == "" {
		urlslug = "blob.core.usgovcloudapi.net"
	}

	// Create a BlockBlobURL object to a blob in the container (we assume the container already exists).
	u, _ := url.Parse(fmt.Sprintf("https://%s.%s/", sa.Account, urlslug))
	credential, err := azblob.NewSharedKeyCredential(sa.Account, sa.AccountKey)
	if err != nil {
		return nil, err
	}
	URL := azblob.NewServiceURL(*u, azblob.NewPipeline(credential, azblob.PipelineOptions{}))
	return &URL, nil
}

func (sa *BlobStorageAccount) ContainerUrl(key string) (*azblob.ContainerURL, error) {
	urlslug := sa.UrlSlug
	if urlslug == "" {
		urlslug = "blob.core.usgovcloudapi.net"
	}

	// Create a BlockBlobURL object to a blob in the container (we assume the container already exists).
	u, _ := url.Parse(fmt.Sprintf("https://%s.%s/%s", sa.Account, urlslug, key))
	credential, err := azblob.NewSharedKeyCredential(sa.Account, sa.AccountKey)
	if err != nil {
		return nil, err
	}
	URL := azblob.NewContainerURL(*u, azblob.NewPipeline(credential, azblob.PipelineOptions{}))
	return &URL, nil
}

type BlobStorage struct {
	Account    string
	AccountKey string
	Container  string
	UrlSlug    string
}

func NewBlobContainer(ctx context.Context, account string, accountKey string, container string, urlslug string) *BlobStorage {
	if urlslug == "" {
		urlslug = "blob.core.usgovcloudapi.net"
	}

	return &BlobStorage{
		Account:    account,
		AccountKey: accountKey,
		Container:  container,
		UrlSlug:    urlslug,
	}
}

func (b *BlobStorage) Upload(ctx context.Context, key string, data io.Reader, tags map[string]string) error {
	blockBlobURL, err := b.BlockBlobUrl(key)
	if err != nil {
		fmt.Printf("Error creating BlockBlobUrl, %v", err)
		return err
	}

	// Perform UploadStreamToBlockBlob
	bufferSize := 2 * 1024 * 1024 // Configure the size of the rotating buffers that are used when uploading
	maxBuffers := 3               // Configure the number of rotating buffers that are used when uploading
	_, err = azblob.UploadStreamToBlockBlob(ctx, data, *blockBlobURL,
		azblob.UploadStreamToBlockBlobOptions{BufferSize: bufferSize, MaxBuffers: maxBuffers})

	// Verify that upload was successful
	if err != nil {
		fmt.Printf("Error uploading, %v", err)
		return err
	}
	return nil
}

func (b *BlobStorage) Exists(ctx context.Context, key string) (bool, error) {
	bURL, err := b.BlobUrl(key)
	if err != nil {
		return false, err
	}

	ac := azblob.BlobAccessConditions{
		LeaseAccessConditions:    azblob.LeaseAccessConditions{},
		ModifiedAccessConditions: azblob.ModifiedAccessConditions{},
	}
	cpk := azblob.ClientProvidedKeyOptions{}

	resp, err := bURL.GetProperties(context.Background(), ac, cpk)
	if err != nil {
		return false, err
	}

	if resp.StatusCode() == 200 {
		return true, nil
	} else {
		return false, nil
	}
}

func (b *BlobStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	blobURL, err := b.BlobUrl(key)
	if err != nil {
		return nil, err
	}

	// Download the blob's contents and verify that it worked correctly
	get, err := blobURL.Download(context.Background(), 0, 0, azblob.BlobAccessConditions{}, false, azblob.ClientProvidedKeyOptions{})
	if err != nil {
		return nil, err
	}
	reader := get.Body(azblob.RetryReaderOptions{})
	return reader, nil
}

func (b *BlobStorage) Delete(ctx context.Context, key string) error {
	blobURL, err := b.BlobUrl(key)
	if err != nil {
		return err
	}

	resp, err := blobURL.Delete(ctx, azblob.DeleteSnapshotsOptionNone, azblob.BlobAccessConditions{})
	if err != nil {
		return err
	}
	if resp.StatusCode() == 202 {
		return nil
	}
	return fmt.Errorf("error deleting blob %v", resp.Status())
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
		Key:  item.Name,
		Tags: b.TagsToMap(item.BlobTags),
		Size: *item.Properties.ContentLength,
		MD5:  string(item.Properties.ContentMD5),
	}
}

func (b *BlobStorage) TagsToMap(tags *azblob.BlobTags) map[string]string {
	rtn := make(map[string]string)
	if tags != nil {
		for _, t := range tags.BlobTagSet {
			rtn[t.Key] = t.Value
		}
	}
	return rtn
}

func (b *BlobStorage) ListNative(ctx context.Context, prefix string) ([]*azblob.BlobItemInternal, []*azblob.BlobPrefix, error) {
	Url, err := b.ContainerUrl()
	if err != nil {
		return nil, nil, err
	}

	marker := azblob.Marker{}
	options := azblob.ListBlobsSegmentOptions{
		Prefix: prefix,
	}

	resp, err := Url.ListBlobsHierarchySegment(ctx, marker, "/", options)

	items := make([]*azblob.BlobItemInternal, len(resp.Segment.BlobItems))
	prefixes := make([]*azblob.BlobPrefix, len(resp.Segment.BlobPrefixes))

	for i, item := range resp.Segment.BlobItems {
		items[i] = &item
	}
	for i, prefix := range resp.Segment.BlobPrefixes {
		prefixes[i] = &prefix
	}

	return items, prefixes, err
}

func (b *BlobStorage) BlobUrl(key string) (*azblob.BlobURL, error) {
	urlslug := b.UrlSlug
	if urlslug == "" {
		urlslug = "blob.core.usgovcloudapi.net"
	}
	// Create a BlockBlobURL object to a blob in the container (we assume the container already exists).
	u, _ := url.Parse(fmt.Sprintf("https://%s.%s/%s/%s", b.Account, urlslug, b.Container, key))
	credential, err := azblob.NewSharedKeyCredential(b.Account, b.AccountKey)
	if err != nil {
		return nil, err
	}
	blobURL := azblob.NewBlobURL(*u, azblob.NewPipeline(credential, azblob.PipelineOptions{}))
	return &blobURL, nil
}

func (b *BlobStorage) BlockBlobUrl(key string) (*azblob.BlockBlobURL, error) {
	urlslug := b.UrlSlug
	if urlslug == "" {
		urlslug = "blob.core.usgovcloudapi.net"
	}

	// Create a BlockBlobURL object to a blob in the container (we assume the container already exists).
	u, _ := url.Parse(fmt.Sprintf("https://%s.%s/%s/%s", b.Account, urlslug, b.Container, key))
	credential, err := azblob.NewSharedKeyCredential(b.Account, b.AccountKey)
	if err != nil {
		return nil, err
	}
	blockBlobURL := azblob.NewBlockBlobURL(*u, azblob.NewPipeline(credential, azblob.PipelineOptions{}))
	return &blockBlobURL, nil
}

func (b *BlobStorage) ContainerUrl() (*azblob.ContainerURL, error) {
	urlslug := b.UrlSlug
	if urlslug == "" {
		urlslug = "blob.core.usgovcloudapi.net"
	}

	// Create a BlockBlobURL object to a blob in the container (we assume the container already exists).
	u, _ := url.Parse(fmt.Sprintf("https://%s.%s/%s", b.Account, urlslug, b.Container))
	credential, err := azblob.NewSharedKeyCredential(b.Account, b.AccountKey)
	if err != nil {
		return nil, err
	}
	URL := azblob.NewContainerURL(*u, azblob.NewPipeline(credential, azblob.PipelineOptions{}))
	return &URL, nil
}
