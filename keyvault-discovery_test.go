package cloudyazure

// import (
// 	"fmt"
// 	"testing"

// 	"github.com/appliedres/cloudy"
// 	"github.com/appliedres/cloudy/testutil"
// 	"github.com/stretchr/testify/assert"
// )

// func TestKeyVaultDiscovery(t *testing.T) {
// 	env := testutil.CreateTestEnvironment()
// 	ctx := cloudy.StartContext()

// 	kvd, err := NewKeyVaultDiscoveryFromEnv(env)
// 	assert.NoError(t, err)
// 	if err != nil {
// 		return
// 	}

// 	all, err := kvd.ListAll(ctx)
// 	assert.NoError(t, err)
// 	if err != nil {
// 		return
// 	}

// 	for _, item := range all {
// 		fmt.Printf("%+v\n", item)
// 	}
// }
