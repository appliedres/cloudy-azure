package cloudyazure

import (
	"context"

	"github.com/appliedres/cloudy/datastore"
)

type AzureCosmosDbDatastore struct {
	url string
	key string
	DB  *Cosmosdb
}

func NewAzureCosmosDb(url string, key string, database string, collection string, v interface{}) *AzureCosmosDbDatastore {
	cosmosDb, _ := NewCosmosdb(database, collection, key, url, v)
	return &AzureCosmosDbDatastore{
		url: url,
		key: key,
		DB:  cosmosDb,
	}
}

func (az *AzureCosmosDbDatastore) Open(ctx context.Context, config interface{}) error {
	return nil
}

func (az *AzureCosmosDbDatastore) Close(ctx context.Context) error {
	// Nothing to do
	return nil
}

// Saves an item into the Elastic Search. This item MUST be JSON data.
// The key is used as the ID for the document and is required to be unique
// for this index
func (az *AzureCosmosDbDatastore) Save(ctx context.Context, item interface{}, key string) error {
	return az.DB.Upsert(key, item)
}

func (az *AzureCosmosDbDatastore) Get(ctx context.Context, key string) (interface{}, error) {
	rtn, err := az.DB.Get(key)
	return rtn, err
}

func (az *AzureCosmosDbDatastore) GetAll(ctx context.Context) ([]interface{}, error) {
	rtn, err := az.DB.GetAll()
	return rtn, err
}

func (az *AzureCosmosDbDatastore) Exists(ctx context.Context, key string) (bool, error) {
	exist, err := az.DB.Exists(key)
	return exist, err
}

func (az *AzureCosmosDbDatastore) Delete(ctx context.Context, key string) error {
	err := az.DB.Remove(key)
	return err
}

func (az *AzureCosmosDbDatastore) Ping(ctx context.Context) bool {
	return az.DB != nil
}

func (az *AzureCosmosDbDatastore) Query(ctx context.Context, query *datastore.SimpleQuery) ([]interface{}, error) {
	return nil, nil
}
