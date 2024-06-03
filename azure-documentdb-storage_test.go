package cloudyazure

import (
	"fmt"
	"testing"
	"time"

	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/datastore"
	"github.com/stretchr/testify/assert"
)

// func CreateCompleteEnvironment(envVar string, PrefixVar string, credentialPrefix string) *cloudy.Environment {
// 	testutil.MustSetTestEnv()
// 	// create a simple env first
// 	tempEnv := cloudy.NewEnvironment(
// 		cloudy.NewHierarchicalEnvironment(
// 			cloudy.NewTieredEnvironment(
// 				cloudy.NewTestFileEnvironmentService(),
// 				cloudy.NewOsEnvironmentService()), ""))
// 	envServiceList := tempEnv.Default(envVar, "test|osenv")
// 	prefix := tempEnv.Get(PrefixVar)

// 	// Split and iterate
// 	envServiceDrivers := strings.Split(envServiceList, "|")

// 	// Create the overall environment
// 	envServices := make([]cloudy.EnvironmentService, len(envServiceDrivers))
// 	for i, svcDriver := range envServiceDrivers {
// 		envSvcInstance, err := cloudy.EnvironmentProviders.NewFromEnvMgrWith(em, svcDriver)
// 		if err != nil {
// 			log.Fatalf("Could not create environment: %v -> %v", svcDriver, err)
// 		}
// 		envServices[i] = envSvcInstance
// 	}

// 	env := cloudy.NewEnvironment(cloudy.NewHierarchicalEnvironment(cloudy.NewTieredEnvironment(envServices...), prefix))
// 	cloudy.SetDefaultEnvironment(env)
// 	return env
// }

func TestCreateOne(t *testing.T) {
	em := cloudy.GetDefaultEnvManager()
	em.LoadSources("test")

	COSMOS_URL := em.GetVar("COSMOS_URL")
	COSMOS_KEY := em.GetVar("COSMOS_KEY")

	ctx := cloudy.StartContext()
	db := NewAzureCosmosDb[TestItem](ctx, COSMOS_URL, COSMOS_KEY, "arkloud", "testitems", "PartitionKey", "PartitionKeyValue", true)
	err := db.Open(ctx, nil)
	assert.NoError(t, err)
	if err != nil {
		panic(err)
	}

	id := cloudy.GenerateId("TEST-", 12)

	exists, err := db.Exists(ctx, id)
	assert.NoError(t, err)
	assert.False(t, exists)

	uvm := &TestItem{
		ID:      id,
		Str:     "Rand name for " + id,
		Time:    time.Now(),
		Integer: 100,
		Double:  1333.3333,
		Tags: map[string]string{
			"Hi": "There",
		},
		Keys: []*NestedItem{
			{Key: "Key1", Value: "Value2"},
			{Key: "Key3", Value: "Value4"},
		},
	}

	err = db.Save(ctx, uvm, uvm.ID)
	assert.NoError(t, err)

	exists2, err := db.Exists(ctx, id)
	assert.NoError(t, err)
	assert.True(t, exists2)

	uvmLoaded, err := db.Get(ctx, id)
	assert.NoError(t, err)
	assert.NotNil(t, uvmLoaded)
	assert.Equal(t, uvm.ID, uvmLoaded.ID)
	assert.Equal(t, uvm.Str, uvmLoaded.Str)

	uvmLoaded.Str = "Updated " + id
	err = db.Save(ctx, &uvmLoaded, uvmLoaded.ID)
	assert.NoError(t, err)

	err = db.DB.Remove(ctx, uvmLoaded.ID)
	assert.NoError(t, err)

	exists3, err := db.Exists(ctx, id)
	assert.NoError(t, err)
	assert.False(t, exists3)
}

func TestCreate100(t *testing.T) {
	em := cloudy.GetDefaultEnvManager()
	em.LoadSources("test")

	COSMOS_URL := em.GetVar("COSMOS_URL")
	COSMOS_KEY := em.GetVar("COSMOS_KEY")

	ctx := cloudy.StartContext()
	db := NewAzureCosmosDb[TestItem](ctx, COSMOS_URL, COSMOS_KEY, "arkloud", "testitems", "PartitionKey", "PartitionKeyValue", true)
	err := db.Open(ctx, nil)
	assert.NoError(t, err)
	if err != nil {
		panic(err)
	}

	all, err := db.GetAll(ctx)
	if err != nil {
		panic(err)
	}

	if len(all) == 0 {
		for i := range 100 {
			id := cloudy.GenerateId("TEST", 12)
			uvm := &TestItem{
				ID:      id,
				Str:     "Rand name for " + id,
				Time:    time.Now(),
				Integer: 100,
				Double:  1333.3333,
				Tags: map[string]string{
					"Hi": "There",
				},
				Keys: []*NestedItem{
					{Key: "Key1", Value: "Value2"},
					{Key: "Index", Value: fmt.Sprintf("%v", i)},
				},
			}

			err = db.Save(ctx, uvm, uvm.ID)
			if err != nil {
				panic(err)
			}
			assert.NoError(t, err)
		}
	}

	// Query
	q := datastore.NewQuery()
	q.Conditions.GreaterThanOrEqual("Integer", "50")

	items, err := db.Query(ctx, q)
	assert.NoError(t, err)
	if err != nil {
		panic(err)
	}
	for _, item := range items {
		assert.NotNil(t, item)
	}
	assert.Greater(t, len(items), 0)

}

type TestItem struct {
	ID      string `json:"id"`
	Time    time.Time
	Integer int
	Double  float64
	Str     string
	Tags    map[string]string
	Keys    []*NestedItem
}

type NestedItem struct {
	Key   string
	Value string
}
