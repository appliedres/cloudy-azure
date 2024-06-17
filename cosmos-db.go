package cloudyazure

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
)

type CosmosObject struct {
	Item interface{}
}

type Cosmosdb struct {
	Database        string
	Container       string
	client          *azcosmos.Client
	dbclient        *azcosmos.DatabaseClient
	containerClient *azcosmos.ContainerClient
	pk              azcosmos.PartitionKey // Cannot figure out how to
	pkField         string
	pkValue         string
	idField         string
	cred            *azcosmos.KeyCredential
}

func NewCosmosdb(ctx context.Context, db string, coll string, key string, endpoint string, idField string, partitionKeyField string, partitionKey string) (*Cosmosdb, error) {
	c := &Cosmosdb{
		Database:  db,
		Container: coll,
		pkValue:   partitionKey,
		pk:        azcosmos.NewPartitionKeyString(partitionKey),
		pkField:   partitionKeyField,
		idField:   idField,
	}

	keyCred, err := azcosmos.NewKeyCredential(key)
	if err != nil {
		return nil, err
	}
	c.cred = &keyCred

	client, err := azcosmos.NewClientWithKey(endpoint, keyCred, &azcosmos.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud:     cloud.AzureGovernment,
			Transport: DefaultTransport,
		},
	})
	if err != nil {
		return nil, err
	}
	c.client = client
	return c, nil
}

func (c *Cosmosdb) CreateOpen(ctx context.Context, onCollectionCreate func(ctx context.Context, c *Cosmosdb) error) error {
	err := c.findOrCreateDatabase(ctx)
	if err != nil {
		return err
	}
	err = c.findOrCreateContainer(ctx, onCollectionCreate)
	return err
}

func (c *Cosmosdb) Healthy(ctx context.Context) error {
	_, err := c.dbclient.Read(ctx, &azcosmos.ReadDatabaseOptions{})
	return err
}

// Find or create database by id
func (c *Cosmosdb) findOrCreateDatabase(ctx context.Context) error {
	dbClient, err := c.client.NewDatabase(c.Database)
	if err != nil {
		return err
	}

	resp, err := dbClient.Read(ctx, &azcosmos.ReadDatabaseOptions{})
	if err != nil && !is404(err) {
		return err
	}
	if resp.DatabaseProperties != nil {
		c.dbclient = dbClient
		return nil
	}

	createResponse, err := c.client.CreateDatabase(ctx, azcosmos.DatabaseProperties{ID: c.Database}, nil)
	if err != nil {
		return err
	}
	c.dbclient = dbClient
	c.Database = createResponse.DatabaseProperties.ID
	return nil
}

// Find or create Container by id
func (c *Cosmosdb) findOrCreateContainer(ctx context.Context, onCollectionCreate func(ctx context.Context, c *Cosmosdb) error) error {
	containerClient, err := c.dbclient.NewContainer(c.Container)
	if err != nil {
		return err
	}

	pkField := c.pkField
	if !strings.HasPrefix(pkField, "/") {
		pkField = "/" + pkField
	}

	idField := c.idField
	if !strings.HasPrefix(idField, "/") {
		idField = "/" + idField
	}

	_, err = containerClient.Read(ctx, &azcosmos.ReadContainerOptions{})

	// Create
	if c.IsStatusCodeNotFound(err) {
		properties := azcosmos.ContainerProperties{
			ID: c.Container,
			PartitionKeyDefinition: azcosmos.PartitionKeyDefinition{
				Paths: []string{pkField},
			},
		}

		createResponse, err := c.dbclient.CreateContainer(ctx, properties, &azcosmos.CreateContainerOptions{})
		if err != nil {
			return err
		}
		c.containerClient = containerClient
		c.Container = createResponse.ContainerProperties.ID

		if onCollectionCreate != nil {
			onCollectionCreate(ctx, c)
		}

		return nil
	}

	if err != nil {
		return err
	}
	c.containerClient = containerClient
	return nil
}

// Get user by given id
func (c *Cosmosdb) Exists(ctx context.Context, id string) (bool, error) {
	_, err := c.containerClient.ReadItem(ctx, c.pk, id, &azcosmos.ItemOptions{})
	if c.IsStatusCodeNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Get user by given id
func (c *Cosmosdb) GetRaw(ctx context.Context, id string) ([]byte, error) {
	resp, err := c.containerClient.ReadItem(ctx, c.pk, id, &azcosmos.ItemOptions{})

	if is404(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	data := resp.Value
	return data, nil
}

// Get user by given id
func (c *Cosmosdb) GetAll(ctx context.Context) ([][]byte, error) {
	var items [][]byte
	query := "SELECT * FROM " + c.Container
	pager := c.containerClient.NewQueryItemsPager(query, c.pk, nil)
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return items, err
		}
		items = append(items, resp.Items...)
	}
	return items, nil
}

// Create user
func (c *Cosmosdb) Add(ctx context.Context, id string, data []byte) error {
	var err error

	data, err = c.AddPK(data)
	if err != nil {
		return err
	}

	_, err = c.containerClient.CreateItem(ctx, c.pk, data, &azcosmos.ItemOptions{})
	return err
}

// Update or insert
func (c *Cosmosdb) Upsert(ctx context.Context, id string, data []byte) error {
	var err error

	data, err = c.AddPK(data)
	if err != nil {
		return err
	}

	_, err = c.containerClient.UpsertItem(ctx, c.pk, data, &azcosmos.ItemOptions{})
	return err
}

// Update or insert
func (c *Cosmosdb) Replace(ctx context.Context, id string, data []byte) error {
	var err error
	// Save to map
	data, err = c.AddPK(data)
	if err != nil {
		return err
	}
	_, err = c.containerClient.ReplaceItem(ctx, c.pk, id, data, &azcosmos.ItemOptions{})
	return err
}

func (c *Cosmosdb) Remove(ctx context.Context, id string) error {
	_, err := c.containerClient.DeleteItem(ctx, c.pk, id, nil)
	if c.IsStatusCodeNotFound(err) {
		return nil
	}
	return err
}

func (c *Cosmosdb) QueryAll(ctx context.Context, query string) ([][]byte, error) {
	var rtn [][]byte
	pager := c.containerClient.NewQueryItemsPager(query, c.pk, &azcosmos.QueryOptions{})
	for pager.More() {
		r, err := pager.NextPage(ctx)
		if err != nil {
			return rtn, err
		}
		rtn = append(rtn, r.Items...)
	}
	return rtn, nil
}

func (c *Cosmosdb) AddPK(data []byte) ([]byte, error) {
	obj := make(map[string]interface{})

	err := json.Unmarshal(data, &obj)
	if err != nil {
		return data, err
	}

	_, pkFound := obj[c.pkField]
	if !pkFound {
		obj[c.pkField] = c.pkValue
	}
	_, idFound := obj["id"]
	if !idFound {
		obj["id"] = obj[c.idField]
	}

	return json.Marshal(obj)
}

func (c *Cosmosdb) IsStatusCodeNotFound(err error) bool {
	if err == nil {
		return false
	}

	var azErr *azcore.ResponseError
	if errors.As(err, &azErr) {
		return azErr.StatusCode == 404
	}
	return false
}
