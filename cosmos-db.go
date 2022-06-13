package cloudyazure

import (
	"encoding/json"
	"fmt"

	"github.com/a8m/documentdb"
	"github.com/appliedres/cloudy"
)

type CosmosObject struct {
	documentdb.Document
	Item interface{}
}

type Cosmosdb struct {
	Database   string
	Collection string
	db         *documentdb.Database
	coll       *documentdb.Collection
	client     *documentdb.DocumentDB
	Model      interface{}
}

func NewCosmosdb(db string, coll string, key string, endpoint string, model interface{}) (*Cosmosdb, error) {
	c := &Cosmosdb{
		Database:   db,
		Collection: coll,
		Model:      model,
	}
	config := documentdb.NewConfig(&documentdb.Key{
		Key: key,
	})
	config.IdentificationHydrator = func(config *documentdb.Config, doc interface{}) {
		fmt.Println("Hydrading")
	}

	c.client = documentdb.New(endpoint, config)

	err := c.findOrDatabase(db)
	if err != nil {
		return nil, err
	}
	err = c.findOrCreateCollection(coll)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Find or create collection by id
func (c *Cosmosdb) findOrCreateCollection(name string) error {

	query := fmt.Sprintf("SELECT * FROM ROOT r WHERE r.id='%s'", name)
	q := &documentdb.Query{
		Query: query,
	}
	colls, err := c.client.QueryCollections(c.db.Self, q)
	if err != nil {
		return err
	}
	if len(colls) == 0 {
		var collKey documentdb.Collection
		collKey.Id = name
		coll, err := c.client.CreateCollection(c.db.Self, collKey)
		if err != nil {
			return err
		}
		c.coll = coll
	} else {
		c.coll = &colls[0]
	}

	return nil
}

// Find or create database by id
func (c *Cosmosdb) findOrDatabase(name string) error {
	query := fmt.Sprintf("SELECT * FROM ROOT r WHERE r.id='%s'", name)
	q := &documentdb.Query{
		Query: query,
	}
	dbs, err := c.client.QueryDatabases(q)
	if err != nil {
		return err
	}
	if len(dbs) == 0 {
		var dbKey documentdb.Database
		dbKey.Id = name
		db, err := c.client.CreateDatabase(dbKey)
		if err != nil {
			return err
		}
		c.db = db
	} else {
		c.db = &dbs[0]
	}

	return nil
}

// Get user by given id
func (c *Cosmosdb) Exists(id string) (bool, error) {
	var items []CosmosObject
	query := fmt.Sprintf("SELECT * FROM ROOT r WHERE r.id='%s'", id)
	q := &documentdb.Query{
		Query: query,
	}
	_, err := c.client.QueryDocuments(c.coll.Self, q, &items)
	if err != nil {
		return false, err
	}
	if len(items) == 0 {
		return false, nil
	}
	return true, nil
}

// Get user by given id
func (c *Cosmosdb) GetRaw(id string) (*CosmosObject, error) {
	var items []CosmosObject
	query := fmt.Sprintf("SELECT * FROM ROOT r WHERE r.id='%s'", id)
	q := &documentdb.Query{
		Query: query,
	}
	_, err := c.client.QueryDocuments(c.coll.Self, q, &items)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	item := items[0]

	// This is a wrapped object
	return &item, nil
}

// Get user by given id
func (c *Cosmosdb) Get(id string) (interface{}, error) {
	var items []CosmosObject
	query := fmt.Sprintf("SELECT * FROM ROOT r WHERE r.id='%s'", id)
	q := &documentdb.Query{
		Query: query,
	}
	_, err := c.client.QueryDocuments(c.coll.Self, q, &items)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	item := items[0].Item

	data, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}

	instance := cloudy.NewInstance(c.Model)
	err = json.Unmarshal(data, instance)
	if err != nil {
		return nil, err
	}

	// This is a wrapped object
	return instance, nil
}

// Get user by given id
func (c *Cosmosdb) GetAll() ([]interface{}, error) {
	var items []CosmosObject
	query := "SELECT * FROM ROOT r"
	q := &documentdb.Query{
		Query: query,
	}
	_, err := c.client.QueryDocuments(c.coll.Self, q, &items)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}

	rtn := make([]interface{}, len(items))
	for i := range items {
		item := items[0].Item

		data, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}

		instance := cloudy.NewInstance(c.Model)
		err = json.Unmarshal(data, instance)
		if err != nil {
			return nil, err
		}

		rtn[i] = instance
	}

	// This is a wrapped object
	return rtn, nil
}

// Create user
func (c *Cosmosdb) Add(id string, v interface{}) error {
	obj := &CosmosObject{
		Item: v,
	}
	obj.Id = id

	_, err := c.client.CreateDocument(c.coll.Self, obj)
	return err
}

// Update or insert
func (c *Cosmosdb) Upsert(id string, item interface{}) error {

	obj := &CosmosObject{
		Item: item,
	}
	obj.Id = id

	_, err := c.client.UpsertDocument(c.coll.Self, obj)
	return err
}

// Update user by id
func (c *Cosmosdb) Update(id string, item interface{}) error {
	var items []CosmosObject
	query := fmt.Sprintf("SELECT * FROM ROOT r WHERE r.id='%s'", id)
	q := &documentdb.Query{
		Query: query,
	}
	_, err := c.client.QueryDocuments(c.coll.Self, q, &items)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	selfId := items[0].Self
	obj := &CosmosObject{
		Item: item,
	}
	obj.Id = items[0].Id

	// TODO? How to get Self? User reflection right now?
	// selfId := c.DocLink(id)
	_, err = c.client.ReplaceDocument(selfId, &item)
	if err != nil {
		return err
	}
	return nil
}

func (c *Cosmosdb) Remove(id string) error {
	found, err := c.GetRaw(id)
	if err != nil {
		return err
	}

	if found != nil {
		_, err = c.client.DeleteDocument(found.Self)
		return err
	}

	return nil
}

// func (c *Cosmosdb) DocLink(id string) string {
// 	return fmt.Sprintf("dbs/%v/colls/%v/docs/%v", c.Database, c.Collection, id)
// }

// User document
type User struct {
	documentdb.Document
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}
