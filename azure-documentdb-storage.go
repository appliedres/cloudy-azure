package cloudyazure

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/datastore"
)

func init() {
	datastore.UntypedJsonDataStoreFactoryProviders.Register(AzureCosmosDB, &CosmosDBFactory{})
}

// FACTORY --------------------------------
const AzureCosmosDB = "azure-cosmosdb"

type CosmosFactoryConfig struct {
	URL      string
	Key      string
	Creds    *AzureCredentials
	Database string
}

type CosmosDBFactory struct {
	config *CosmosFactoryConfig
}

func (c *CosmosDBFactory) Create(cfg interface{}) (datastore.UntypedJsonDataStoreFactory, error) {
	cloudy.Info(context.Background(), "CosmosDBFactory Create")
	sec := cfg.(*CosmosFactoryConfig)
	if sec == nil {
		return nil, cloudy.ErrInvalidConfiguration
	}
	return &CosmosDBFactory{
		config: cfg.(*CosmosFactoryConfig),
	}, nil
}

func (c *CosmosDBFactory) FromEnv(env *cloudy.Environment) (interface{}, error) {
	// creds := GetAzureCredentialsFromEnv(env)

	cfg := &CosmosFactoryConfig{
		URL:      env.Force("AZ_COSMOS_URL"),
		Key:      env.Force("AZ_COSMOS_KEY"),
		Database: env.Default("AZ_COSMOS_DB", "arkloud"),
		// Creds:    &creds,
	}
	return cfg, nil
}

func (c *CosmosDBFactory) CreateJsonDatastore(ctx context.Context, typename string, prefix string, idField string) datastore.UntypedJsonDataStore {
	return NewAzureCosmosDb(ctx, c.config.URL, c.config.Key, c.config.Database, typename, idField, typename, typename)
}

// DATASTORE -------------------------------

type AzureCosmosDbDatastore struct {
	url      string
	key      string
	DB       *Cosmosdb
	onCreate func(ctx context.Context, ds datastore.UntypedJsonDataStore) error
}

func NewAzureCosmosDb(ctx context.Context, url string, key string, database string, collection string, idField string, pkField string, pkValue string) *AzureCosmosDbDatastore {
	cosmosDb, _ := NewCosmosdb(ctx, database, collection, key, url, idField, pkField, pkValue)
	return &AzureCosmosDbDatastore{
		url: url,
		key: key,
		DB:  cosmosDb,
	}
}

func (az *AzureCosmosDbDatastore) Open(ctx context.Context, config interface{}) error {
	return az.DB.CreateOpen(ctx, func(ctx context.Context, c *Cosmosdb) error {
		return nil
	})
}

func (az *AzureCosmosDbDatastore) Close(ctx context.Context) error {
	// Nothing to do
	return nil
}

// Saves an item into the Elastic Search. This item MUST be JSON data.
// The key is used as the ID for the document and is required to be unique
// for this index
func (az *AzureCosmosDbDatastore) Save(ctx context.Context, data []byte, key string) error {
	return az.DB.Upsert(ctx, key, data)
}

func (az *AzureCosmosDbDatastore) SaveAll(ctx context.Context, b [][]byte, s []string) error {
	return cloudy.Error(ctx, "Not implemented")
}

func (az *AzureCosmosDbDatastore) Get(ctx context.Context, id string) ([]byte, error) {
	data, err := az.DB.GetRaw(ctx, id)
	return data, err
}

func (az *AzureCosmosDbDatastore) GetAll(ctx context.Context) ([][]byte, error) {
	cloudy.Info(ctx, "AzureCosmosDbDatastore.GetAll")
	results, err := az.DB.GetAll(ctx)
	return results, err
}

func (az *AzureCosmosDbDatastore) Exists(ctx context.Context, key string) (bool, error) {
	exist, err := az.DB.Exists(ctx, key)
	return exist, err
}

func (az *AzureCosmosDbDatastore) Delete(ctx context.Context, key string) error {
	err := az.DB.Remove(ctx, key)
	return err
}

func (az *AzureCosmosDbDatastore) DeleteAll(ctx context.Context, key []string) error {
	return cloudy.Error(ctx, "Not implemented")
}

func (az *AzureCosmosDbDatastore) Ping(ctx context.Context) bool {
	err := az.DB.Healthy(ctx)
	return err == nil
}

func (az *AzureCosmosDbDatastore) Query(ctx context.Context, query *datastore.SimpleQuery) ([][]byte, error) {
	cloudy.Info(ctx, "AzureCosmosDbDatastore.Query")

	sql := new(CosmosDbQueryConverter).Convert(query, "c")

	results, err := az.DB.QueryAll(ctx, sql)
	return results, err
}

func (az *AzureCosmosDbDatastore) QueryAndUpdate(ctx context.Context, ds *datastore.SimpleQuery, f func(context.Context, [][]byte) ([][]byte, error)) ([][]byte, error) {
	return nil, cloudy.Error(ctx, "Not implemented")
}

func (az *AzureCosmosDbDatastore) QueryAsMap(ctx context.Context, ds *datastore.SimpleQuery) ([]map[string]any, error) {
	return nil, cloudy.Error(ctx, "Not implemented")
}

func (az *AzureCosmosDbDatastore) QueryTable(ctx context.Context, ds *datastore.SimpleQuery) ([][]interface{}, error) {
	return nil, cloudy.Error(ctx, "Not implemented")
}

func (az *AzureCosmosDbDatastore) OnCreate(fn func(ctx context.Context, ds datastore.UntypedJsonDataStore) error) {
	az.onCreate = fn
}

type CosmosDbQueryConverter struct {
	table string
}

func (qc *CosmosDbQueryConverter) Convert(c *datastore.SimpleQuery, table string) string {
	qc.table = table
	where := qc.ConvertConditionGroup(c.Conditions)
	sort := qc.ConvertSort(c.SortBy)

	sql := qc.ConvertSelect(c, table)
	if where != "" {
		sql = sql + " WHERE " + where
	}
	if sort != "" {
		sql = sql + " ORDER BY " + sort
	}
	return sql
}

func (qc *CosmosDbQueryConverter) ConvertSelect(c *datastore.SimpleQuery, table string) string {
	str := "SELECT "
	if len(c.Colums) == 0 {
		str += " * "
	} else {
		var jsonQuery []string
		for _, col := range c.Colums {
			jsonQuery = append(jsonQuery, fmt.Sprintf("data ->> '%v' as \"%v\"", col, col))
		}
		str += strings.Join(jsonQuery, ", ")
	}

	if c.Size > 0 {
		str = fmt.Sprintf("%v LIMIT %v", str, c.Size)
	}

	if c.Offset > 0 {
		str = fmt.Sprintf("%v OFFSET %v", str, c.Offset)
	}

	str += " FROM " + table
	return str
}

func (qc *CosmosDbQueryConverter) ConvertSort(sortbys []*datastore.SortBy) string {
	if len(sortbys) == 0 {
		return ""
	}
	var sorts []string
	for _, sortBy := range sortbys {
		sort := qc.ConvertASort(sortBy)
		if sort != "" {
			sorts = append(sorts, sort)
		}
	}
	return strings.Join(sorts, ", ")
}
func (qc *CosmosDbQueryConverter) ConvertASort(c *datastore.SortBy) string {
	if c.Descending {
		return c.Field + " DESC"
	} else {
		return c.Field + "ASC"
	}
}

func (qc *CosmosDbQueryConverter) ConvertCondition(c *datastore.SimpleQueryCondition) string {
	switch c.Type {
	case "eq":
		return fmt.Sprintf("%v  = '%v'", qc.ToColumnName(c.Data[0]), c.Data[1])
	case "neq":
		return fmt.Sprintf("%v  != '%v'", qc.ToColumnName(c.Data[0]), c.Data[1])
	case "between":
		return fmt.Sprintf("%v BETWEEN %v AND %v", qc.ToColumnName(c.Data[0]), c.Data[1], c.Data[2])
	case "lt":
		return fmt.Sprintf("%v < %v", qc.ToColumnName(c.Data[0]), c.Data[1])
	case "lte":
		return fmt.Sprintf("%v  <= %v", qc.ToColumnName(c.Data[0]), c.Data[1])
	case "gt":
		return fmt.Sprintf("%v  > %v", qc.ToColumnName(c.Data[0]), c.Data[1])
	case "gte":
		return fmt.Sprintf("%v  >= %v", qc.ToColumnName(c.Data[0]), c.Data[1])
	case "before":
		val := c.GetDate("value")
		if !val.IsZero() {
			timestr := val.UTC().Format(time.RFC3339)
			return fmt.Sprintf("%v < '%v'", qc.ToColumnName(c.Data[0]), timestr)
		}
	case "after":
		val := c.GetDate("value")
		if !val.IsZero() {
			timestr := val.UTC().Format(time.RFC3339)
			return fmt.Sprintf("%v > '%v'", qc.ToColumnName(c.Data[0]), timestr)
		}
	case "?":
		return fmt.Sprintf("(data->>'%v')::numeric  ? '%v'", qc.ToColumnName(c.Data[0]), c.Data[1])
	case "contains":
		return fmt.Sprintf("ARRAY_CONTAINS(%v, %v)", qc.ToColumnName(c.Data[0]), c.Data[1])
	case "includes":
		values := c.GetStringArr("value")
		var xformed []string
		for _, v := range values {
			xformed = append(xformed, fmt.Sprintf("'%v'", v))
		}
		if values != nil {
			return fmt.Sprintf("%v in (%v)", qc.ToColumnName(c.Data[0]), strings.Join(xformed, ","))
		}
	case "null":
		return fmt.Sprintf("IS_NULL(%v)", qc.ToColumnName(c.Data[0]))
	}
	return "UNKNOWN"
}

func (qc *CosmosDbQueryConverter) ConvertConditionGroup(cg *datastore.SimpleQueryConditionGroup) string {
	if len(cg.Conditions) == 0 && len(cg.Groups) == 0 {
		return ""
	}

	var conditionStr []string
	for _, c := range cg.Conditions {
		conditionStr = append(conditionStr, qc.ConvertCondition(c))
	}
	for _, c := range cg.Groups {
		result := qc.ConvertConditionGroup(c)
		if result != "" {
			conditionStr = append(conditionStr, "( "+result+" )")
		}
	}
	return strings.Join(conditionStr, " "+cg.Operator+" ")
}

func (qc *CosmosDbQueryConverter) ToColumnName(name string) string {
	return qc.table + "." + name
}
