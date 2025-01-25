package storage

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/appliedres/cloudy"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy/datastore"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type DebugTransporter struct {
	client http.Client
}

func (d *DebugTransporter) Do(req *http.Request) (*http.Response, error) {
	return d.client.Do(req)
}

func getCert(url string) ([]byte, error) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	// Send an HTTP GET request
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

type CosmosdbStrategy struct {
}

func (s *CosmosdbStrategy) WaitUntilReady(ctx context.Context, target wait.StrategyTarget) error {
	log.Println("Waiting on CosmosDB container to be ready. Expect about 2m.")

	timeout := time.Minute * 5
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pollInterval := time.Second * 10

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
			host, err := target.Host(ctx)
			if err != nil {
				return err
			}
			mappedPort, err := target.MappedPort(ctx, "8081")
			if err != nil {
				panic(err)
			}
			endpoint := fmt.Sprintf("https://%s:%s/", host, mappedPort.Port())
			url := fmt.Sprintf("%v_explorer/emulator.pem", endpoint)
			log.Printf("Checking %v ... time elapsed: %v\n", url, time.Since(start))

			cert, err := getCert(url)
			if err == nil {
				certs := x509.NewCertPool()
				certs.AppendCertsFromPEM(cert)
				http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{
					RootCAs: certs,
				}

				cloudyazure.DefaultTransport = &DebugTransporter{
					client: *http.DefaultClient,
				}
				return nil
			}
		}
	}
}

func TestCosmosLocalDBFactory(t *testing.T) {
	key := "C2y6yDjf5/R+ob0N8A7Cgv30VRDJIWEHLM+4QDU5DE2nQ9nDuVTqobD4b8mGGyPMbIZnqyMsEcaGQy67XIw/Jw=="
	strategy := &CosmosdbStrategy{}
	// endpoint := "https://localhost:8081/"
	ctx := cloudy.StartContext()
	req := testcontainers.ContainerRequest{
		Image: "mcr.microsoft.com/cosmosdb/linux/azure-cosmos-emulator:latest",
		ExposedPorts: []string{
			"8081/tcp",
			"10250/tcp",
			"10251/tcp",
			"10252/tcp",
			"10253/tcp",
			"10254/tcp",
			"10255/tcp",
		},
		// WaitingFor: wait.ForLog("11/11 partitions"),
		WaitingFor: strategy,
	}

	// docker run \
	// --interactive \
	// --tty \
	// mcr.microsoft.com/cosmosdb/linux/azure-cosmos-emulator:latest

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})

	if err != nil {
		log.Fatalf("Could not start cosmosdb: %s", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			log.Fatalf("Could not stop cosmosdb: %s", err)
		}
	}()

	ip, err := container.Host(ctx)
	if err != nil {
		panic(err)
	}

	mappedPort, err := container.MappedPort(ctx, "8081")
	if err != nil {
		panic(err)
	}

	endpoint := fmt.Sprintf("https://%s:%s/", ip, mappedPort.Port())
	ui := fmt.Sprintf("%v_explorer/index.html", endpoint)

	fmt.Printf("Endpoint : %v\n", endpoint)
	fmt.Printf("UI : %v\n", ui)

	menv := cloudy.NewMapEnvironment()
	menv.Set("DATATYPE_DRIVER", AzureCosmosDB)
	menv.Set("DATATYPE_AZ_COSMOS_URL", endpoint)
	menv.Set("DATATYPE_AZ_COSMOS_KEY", key)

	// SSL BROKE
	env := cloudy.NewEnvironment(cloudy.NewHierarchicalEnvironment(menv))
	cloudy.SetDefaultEnvironment(env)

	envSegment := env.Segment("DATATYPE")
	ds, err := datastore.CreateJsonDatastore[datastore.TestItem](ctx, "testfactory", "ark", "id", envSegment)
	if err != nil {
		panic(err)
	}

	assert.NotNil(t, ds)
	assert.NotNil(t, ds)
	err = ds.Open(ctx, nil)
	if err != nil {
		panic(err)
	}
	datastore.JsonDataStoreTest(t, ctx, ds)

}

func CreateCompleteEnvironment(envVar string, PrefixVar string, credentialPrefix string) *cloudy.Environment {
	testutil.MustSetTestEnv()
	// create a simple env first
	tempEnv := cloudy.NewEnvironment(
		cloudy.NewHierarchicalEnvironment(
			cloudy.NewTieredEnvironment(
				cloudy.NewTestFileEnvironmentService(),
				cloudy.NewOsEnvironmentService()), ""))
	envServiceList := tempEnv.Default(envVar, "test|osenv")
	prefix := tempEnv.Get(PrefixVar)

	// Split and iterate
	envServiceDrivers := strings.Split(envServiceList, "|")

	// Create the overall environment
	envServices := make([]cloudy.EnvironmentService, len(envServiceDrivers))
	for i, svcDriver := range envServiceDrivers {
		envSvcInstance, err := cloudy.EnvironmentProviders.NewFromEnvWith(tempEnv, svcDriver)
		if err != nil {
			log.Fatalf("Could not create environment: %v -> %v", svcDriver, err)
		}
		envServices[i] = envSvcInstance
	}

	env := cloudy.NewEnvironment(cloudy.NewHierarchicalEnvironment(cloudy.NewTieredEnvironment(envServices...), prefix))
	cloudy.SetDefaultEnvironment(env)
	return env
}

func TestCosmosDBFactory(t *testing.T) {
	menv := cloudy.NewMapEnvironment()
	menv.Set("DATATYPE_DRIVER", AzureCosmosDB)
	menv.Set("DATATYPE_AZ_COSMOS_URL", "testurl")
	menv.Set("DATATYPE_AZ_COSMOS_KEY", base64.StdEncoding.EncodeToString([]byte("super-secret-key")))

	env := cloudy.NewEnvironment(cloudy.NewHierarchicalEnvironment(menv))
	cloudy.SetDefaultEnvironment(env)

	envSegment := env.Segment("DATATYPE")

	factory, err := datastore.UntypedJsonDataStoreFactoryProviders.NewFromEnv(envSegment, "DRIVER")
	if err != nil {
		panic(err)
	}

	ctx := cloudy.StartContext()
	ds := factory.CreateJsonDatastore(ctx, "testfactory", "ark", "ID")
	assert.NotNil(t, ds)

	typedds, err := datastore.CreateJsonDatastore[TestItem](ctx, "testfactory", "ark", "ID", envSegment)
	if err != nil {
		panic(err)
	}
	assert.NotNil(t, typedds)

}

func TestCosmosDBCreateOne(t *testing.T) {
	env := CreateCompleteEnvironment("ARKLOUD_ENV", "", "")
	env = env.Segment("DATATYPE")

	COSMOS_URL := env.Force("AZ_COSMOS_URL")
	COSMOS_KEY := env.Force("AZ_COSMOS_KEY")

	ctx := cloudy.StartContext()
	db := datastore.NewTypedStore[TestItem](NewAzureCosmosDb(ctx, COSMOS_URL, COSMOS_KEY, "arkloud", "testitems", "ID", "PartitionKey", "PartitionKeyValue"))
	err := db.Open(ctx, nil)
	assert.NoError(t, err)
	if err != nil {
		panic(err)
	}

	id := cloudy.GenerateId("TEST-", 12)

	exists, err := db.Exists(ctx, id)
	assert.NoError(t, err)
	assert.False(t, exists)

	item := &TestItem{
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

	err = db.Save(ctx, item, item.ID)
	assert.NoError(t, err)

	exists2, err := db.Exists(ctx, id)
	assert.NoError(t, err)
	assert.True(t, exists2)

	loaded, err := db.Get(ctx, id)
	assert.NoError(t, err)
	assert.NotNil(t, loaded)
	assert.Equal(t, item.ID, loaded.ID)
	assert.Equal(t, item.Str, loaded.Str)

	loaded.Str = "Updated " + id
	err = db.Save(ctx, loaded, loaded.ID)
	assert.NoError(t, err)

	err = db.Delete(ctx, loaded.ID)
	assert.NoError(t, err)

	exists3, err := db.Exists(ctx, id)
	assert.NoError(t, err)
	assert.False(t, exists3)
}

func TestCosmosDBCreate100(t *testing.T) {
	env := CreateCompleteEnvironment("ARKLOUD_ENV", "", "")
	env = env.Segment("DATATYPE")

	COSMOS_URL := env.Force("AZ_COSMOS_URL")
	COSMOS_KEY := env.Force("AZ_COSMOS_KEY")

	ctx := cloudy.StartContext()
	db := datastore.NewTypedStore[TestItem](NewAzureCosmosDb(
		ctx, COSMOS_URL, COSMOS_KEY, "arkloud", "testitems", "ID", "PartitionKey", "PartitionKeyValue"))
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
			item := &TestItem{
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

			err = db.Save(ctx, item, item.ID)
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

func TestCosmosDBDS(t *testing.T) {
	CreateCompleteEnvironment("", "", "")

	ctx := cloudy.StartContext()
	ds, err := datastore.CreateJsonDatastore[datastore.TestItem](ctx, "dstestitem", "ark", "id", cloudy.DefaultEnvironment)
	if err != nil {
		panic(err)
	}
	assert.NotNil(t, ds)
	err = ds.Open(ctx, nil)
	if err != nil {
		panic(err)
	}
	datastore.JsonDataStoreTest(t, ctx, ds)
}

type TestItem struct {
	ID      string
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
