package cloudyazure

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/datastore"
)

type AzureCosmosDbDatastore[T any] struct {
	url string
	key string
	DB  *Cosmosdb
}

func NewAzureCosmosDb[T any](ctx context.Context, url string, key string, database string, collection string, pkField string, pkValue string, addPK bool) *AzureCosmosDbDatastore[T] {
	cosmosDb, _ := NewCosmosdb(ctx, database, collection, key, url, pkField, pkValue, addPK)
	return &AzureCosmosDbDatastore[T]{
		url: url,
		key: key,
		DB:  cosmosDb,
	}
}

func (az *AzureCosmosDbDatastore[T]) Open(ctx context.Context, config interface{}) error {
	return az.DB.CreateOpen(ctx)
}

func (az *AzureCosmosDbDatastore[T]) Close(ctx context.Context) error {
	// Nothing to do
	return nil
}

// Saves an item into the Elastic Search. This item MUST be JSON data.
// The key is used as the ID for the document and is required to be unique
// for this index
func (az *AzureCosmosDbDatastore[T]) Save(ctx context.Context, item *T, key string) error {
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}

	return az.DB.Upsert(ctx, key, data)
}

func (az *AzureCosmosDbDatastore[T]) fromBytes(data []byte) (T, error) {
	var zero T
	v, err := cloudy.NewT[T]()
	if err != nil {
		return zero, err
	}
	err = json.Unmarshal(data, &v)
	if err != nil {
		return zero, err
	}
	return v, err
}

func (az *AzureCosmosDbDatastore[T]) Get(ctx context.Context, id string) (T, error) {
	var zero T
	data, err := az.DB.GetRaw(ctx, id)
	if err != nil {
		return zero, err
	}
	return az.fromBytes(data)
}

func (az *AzureCosmosDbDatastore[T]) GetAll(ctx context.Context) ([]T, error) {
	cloudy.Info(ctx, "AzureCosmosDbDatastore.GetAll")

	results, err := az.DB.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	rtn := make([]T, len(results))
	for i, data := range results {
		obj, err := az.fromBytes(data)
		if err != nil {
			return rtn, err
		}
		rtn[i] = obj
	}
	return rtn, err
}

func (az *AzureCosmosDbDatastore[T]) Exists(ctx context.Context, key string) (bool, error) {
	exist, err := az.DB.Exists(ctx, key)
	return exist, err
}

func (az *AzureCosmosDbDatastore[T]) Delete(ctx context.Context, key string) error {
	err := az.DB.Remove(ctx, key)
	return err
}

func (az *AzureCosmosDbDatastore[T]) Ping(ctx context.Context) bool {
	err := az.DB.Healthy(ctx)
	return err == nil
}

func (az *AzureCosmosDbDatastore[T]) Query(ctx context.Context, query *datastore.SimpleQuery) ([]T, error) {
	sql := new(CosmosDbQueryConverter).Convert(query, "c")

	cloudy.Info(ctx, "AzureCosmosDbDatastore.Query")
	results, err := az.DB.QueryAll(ctx, sql)
	if err != nil {
		return nil, err
	}
	rtn := make([]T, len(results))
	for i, data := range results {
		obj, err := az.fromBytes(data)
		if err != nil {
			return rtn, err
		}
		rtn[i] = obj
	}
	return rtn, err
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
		return fmt.Sprintf("%v IS NULL", qc.ToColumnName(c.Data[0]))
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
