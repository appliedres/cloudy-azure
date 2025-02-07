package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
	"github.com/appliedres/cloudy"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy/storage"
)

func init() {
	storage.ObjectStorageProviders.Register(AzureBlob, &AzureBlobStorageFactory{})
}

type AzureBlobStorageFactory struct{}

func (f *AzureBlobStorageFactory) Create(cfg interface{}) (storage.ObjectStorageManager, error) {
	azCfg := cfg.(*AzureStorageAccount)
	if azCfg == nil {
		return nil, cloudy.ErrInvalidConfiguration
	}
	return NewBlobStorageAccount(context.Background(), azCfg.AccountName, azCfg.AccountKey, azCfg.UrlSlug)
}

func (f *AzureBlobStorageFactory) FromEnv(env *cloudy.Environment) (interface{}, error) {
	cfg := &AzureStorageAccount{}
	cfg.AccountName = env.Force("AZ_ACCOUNT")
	cfg.AccountKey = env.Force("AZ_ACCOUNT_KEY")
	return cfg, nil
}

// ObjectStorageManager  {
type BlobStorageAccount struct {
	Account string
	// AccountKey string
	// UrlSlug    string
	Client *azblob.Client
}

func storageServiceUrl(accountName string, serviceUrl string, urlslug string) string {
	var rtnUrl string
	if serviceUrl != "" {
		rtnUrl = serviceUrl
	} else if strings.HasPrefix(urlslug, "http") {
		rtnUrl = urlslug
	} else {
		if urlslug == "" {
			urlslug = "blob.core.usgovcloudapi.net"
		}
		rtnUrl = fmt.Sprintf("https://%s.%s/", accountName, urlslug)
	}
	return rtnUrl
}

func storageSlug(cloudName string) string {
	cloudNameFixed := cloudyazure.FixCloudName(cloudName)

	switch cloudNameFixed {
	// Default to the government cloud
	case "":
		return "core.usgovcloudapi.net"
	case cloudyazure.CloudUSGovernment:
		return "core.usgovcloudapi.net"
	case cloudyazure.CloudPublic:
		return "core.windows.net"
	default:
		return ""
	}
}

func acctStorageUrl(acct *AzureStorageAccount) string {
	var rtnUrl string
	if acct.ServiceURL != "" {
		rtnUrl = acct.ServiceURL
	} else if strings.HasPrefix(acct.UrlSlug, "http") {
		rtnUrl = acct.UrlSlug
	} else {
		urlSlug := acct.UrlSlug
		if urlSlug == "" {
			if acct.Cloud != "" {
				urlSlug = fmt.Sprintf("blob.%v", storageSlug(acct.Cloud))
			} else {
				urlSlug = "blob.core.usgovcloudapi.net"
			}
		}
		rtnUrl = fmt.Sprintf("https://%s.%s/", acct.AccountName, urlSlug)
	}
	return rtnUrl
}

func NewBlobStorageAccount2(ctx context.Context, acct *AzureStorageAccount) (*BlobStorageAccount, error) {
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

	return &BlobStorageAccount{
		Account: acct.AccountName,
		// AccountKey: acct,
		// UrlSlug:    urlslug,
		Client: client,
	}, nil
}

func NewBlobStorageAccount(ctx context.Context, account string, accountKey string, urlslug string) (*BlobStorageAccount, error) {
	if urlslug == "" {
		urlslug = "blob.core.usgovcloudapi.net"
	}

	var serviceUrl string
	if strings.HasPrefix(urlslug, "http") {
		serviceUrl = urlslug
	} else {
		serviceUrl = fmt.Sprintf("https://%s.%s/", account, urlslug)
	}

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
		Account: account,
		// AccountKey: accountKey,
		// UrlSlug:    urlslug,
		Client: service,
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
				Tags: cloudyazure.FromStrPointerMap(containerItem.Metadata),
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
	key = cloudyazure.SanitizeName(key)

	containerClient := sa.Client.ServiceClient().NewContainerClient(key)

	return NewBlobContainerFrom(ctx, containerClient), nil
}

func (sa *BlobStorageAccount) GetItem(ctx context.Context, key string) (*storage.StorageArea, error) {
	key = cloudyazure.SanitizeName(key)

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
				Tags: cloudyazure.FromStrPointerMap(items[0].Metadata),
			}, nil

		}
	}

	return nil, nil
}

func (sa *BlobStorageAccount) Create(ctx context.Context, key string, openToPublic bool, tags map[string]string) (storage.ObjectStorage, error) {

	opts := &azblob.CreateContainerOptions{
		Metadata: cloudyazure.ToStrPointerMap(tags),
	}

	if openToPublic {
		opts.Access = to.Ptr(azblob.PublicAccessTypeBlob)
	}

	_, err := sa.Client.CreateContainer(ctx, key, opts)
	if err != nil {
		return nil, err
	}

	// Created
	key = cloudyazure.SanitizeName(key)
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

	container = cloudyazure.SanitizeName(container)
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
		Metadata: cloudyazure.ToStrPointerMap(tags),
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

func (b *BlobStorage) GenUploadURL(ctx context.Context, key string) (string, error) {

	expiryTime := time.Now().UTC().Add(1 * time.Hour) // TODO: make dynamic

	c := b.Client.NewBlobClient(key)
	url, err := c.GetSASURL(sas.BlobPermissions{Write: true}, expiryTime, nil)
	if err != nil {
		return "", err
	}

	return url, nil
}

func (b *BlobStorage) GenDownloadURL(ctx context.Context, key string) (string, error) {
	// Set expiry time for the SAS token
	expiryTime := time.Now().UTC().Add(1 * time.Hour) // TODO: make dynamic

	// Create a new Blob client for the specified key (blob name)
	c := b.Client.NewBlobClient(key)

	// Generate the SAS URL with read permissions
	url, err := c.GetSASURL(sas.BlobPermissions{Read: true}, expiryTime, nil)
	if err != nil {
		return "", err
	}

	return url, nil
}

func GenerateUserDelegationSAS(ctx context.Context, creds *cloudyazure.AzureCredentials, storageAccountName, containerName string, validFor time.Duration, permissions sas.ContainerPermissions) (string, error) {
	cred, err := azidentity.NewClientSecretCredential(creds.TenantID, creds.ClientID, creds.ClientSecret, nil)
	if err != nil {
		return "", fmt.Errorf("failed to authenticate with Azure AD: %w", err)
	}

	blobClient, err := azblob.NewClient(fmt.Sprintf("https://%s.blob.core.usgovcloudapi.net", storageAccountName), cred, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create service client: %w", err)
	}

	keyStart := time.Now().UTC().Add(-5 * time.Minute) // Allow some clock skew
	keyExpiry := time.Now().UTC().Add(1 * time.Hour)  // 1-hour validity

	keyInfo := service.KeyInfo{
		Start:    to.Ptr(keyStart.Format(time.RFC3339)),
		Expiry:   to.Ptr(keyExpiry.Format(time.RFC3339)),
	}
	userDelegationCred, err := blobClient.ServiceClient().GetUserDelegationCredential(ctx, keyInfo, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get user delegation credential: %w", err)
	}

	sasQueryParams, err := sas.BlobSignatureValues{
		ContainerName: containerName,
		Protocol:   sas.ProtocolHTTPS,
		StartTime:  keyStart,
		ExpiryTime: keyExpiry,
		Permissions: permissions.String(),
		Version:    "2022-11-02",
	}.SignWithUserDelegation(userDelegationCred)

	if err != nil {
		return "", fmt.Errorf("failed to generate SAS token: %w", err)
	}

	sasURL := fmt.Sprintf("https://%s.blob.core.usgovcloudapi.net/%s?%s",
		storageAccountName, containerName, sasQueryParams.Encode())

	return sasURL, nil
}