package cloudyazure

// import (
// 	"fmt"
// 	"testing"

// 	"github.com/appliedres/cloudy"
// 	"github.com/appliedres/cloudy/testutil"
// 	"github.com/stretchr/testify/assert"
// )

// func TestKeyVaultDiscovery(t *testing.T) {
// 	em := testutil.CreateTestEnvMgr()
// 	ctx := cloudy.StartContext()

// 	kvd, err := NewKeyVaultDiscoveryFromEnvMgr(env)
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
