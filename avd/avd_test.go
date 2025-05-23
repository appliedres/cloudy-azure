package avd

// Import key modules.
import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/appliedres/cloudy"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy/testutil"
	"github.com/stretchr/testify/assert"
)

type Config struct {
	Upn                             string
	UserObjectId                    string
	SessionHost                     string
	HostPool                        string
	DesktopVirtualizationUserRoleId string
	VirtualMachineUserLoginRoleId   string
	ResourceGroupName               string
}

var (
	ctx            context.Context = cloudy.StartContext()
	err            error
	tenantId       string
	clientId       string
	clientSecret   string
	subscriptionId string
	avd            *AzureVirtualDesktopManager
	testConfig     Config
	vaultUrl       string
)

// Broken tests marked with _

func initAVD() error {
	testutil.MustSetTestEnv()
	// _ = testutil.LoadEnv("test.env")
	env := cloudy.CreateEnvironment()

	tenantId = env.Force("AZ_TENANT_ID", "")
	clientId = env.Force("AZ_CLIENT_ID", "")
	clientSecret = env.Force("AZ_CLIENT_SECRET", "")
	subscriptionId = env.Force("AZ_SUBSCRIPTION_ID", "")
	vaultUrl = env.Force("AZ_VAULT_URL", "")

	creds := cloudyazure.AzureCredentials{
		TenantID:     tenantId,
		ClientID:     clientId,
		ClientSecret: clientSecret,
		Region:       "usgovvirginia",
	}

	config := AzureVirtualDesktopManagerConfig{}

	avd, err = NewAzureVirtualDesktopManager(ctx, "unit_test", &creds, &config)
	if err != nil {
		return err
	}

	cred, err := cloudyazure.GetAzureClientSecretCredential(*avd.Credentials)
	if err != nil {
		return err
	}

	client, err := azsecrets.NewClient(vaultUrl, cred, nil)
	if err != nil {
		return err
	}

	res, err := client.GetSecret(ctx, "avd-test-desktop-virtualization-user-role-id", "", nil)
	if err != nil {
		return err
	}
	testConfig.DesktopVirtualizationUserRoleId = *res.Value

	res, err = client.GetSecret(ctx, "avd-test-hostpool", "", nil)
	if err != nil {
		return err
	}
	testConfig.HostPool = *res.Value

	res, err = client.GetSecret(ctx, "avd-test-resource-group", "", nil)
	if err != nil {
		return err
	}
	testConfig.ResourceGroupName = *res.Value

	res, err = client.GetSecret(ctx, "avd-test-session-host", "", nil)
	if err != nil {
		return err
	}
	testConfig.SessionHost = *res.Value

	res, err = client.GetSecret(ctx, "avd-test-upn", "", nil)
	if err != nil {
		return err
	}
	testConfig.Upn = *res.Value

	res, err = client.GetSecret(ctx, "avd-test-user-object-id", "", nil)
	if err != nil {
		return err
	}
	testConfig.UserObjectId = *res.Value

	res, err = client.GetSecret(ctx, "avd-test-user-object-id", "", nil)
	if err != nil {
		return err
	}
	testConfig.VirtualMachineUserLoginRoleId = *res.Value

	return nil
}

func TestValidAVDEnvironment(t *testing.T) {
	err = initAVD()
	assert.Nil(t, err)
}

func _TestRetrieveRegistrationToken(t *testing.T) {
	var (
		hostpools   []*armdesktopvirtualization.HostPool
		sessionHost *string
		regToken    *string
	)
	err = initAVD()
	assert.Nil(t, err)

	hostpools, err = avd.listHostPools(ctx, nil)
	assert.Nil(t, err)
	assert.NotZero(t, len(hostpools))

	for i := 1; i < len(hostpools); i++ {
		sessionHosts, err := avd.ListSessionHosts(ctx, *hostpools[i].Name)
		assert.GreaterOrEqual(t, len(sessionHosts), 0)
		assert.Nil(t, err)
	}

	firstHostpool, err := avd.FindFirstAvailableHostPool(ctx, testConfig.Upn)
	assert.Nil(t, err)
	assert.NotEmpty(t, firstHostpool.Name)

	if err == nil {
		regToken, err = avd.RetrieveRegistrationToken(ctx, *firstHostpool.Name)
		assert.Nil(t, err)
		assert.NotEmpty(t, regToken)
	}

	if err == nil {
		sessionHost, err = avd.getAvailableSessionHost(ctx, *firstHostpool.Name)
		assert.Nil(t, err)
		assert.NotEmpty(t, sessionHost)
	}

	if err == nil {
		err = avd.AssignSessionHost(ctx, *firstHostpool.Name, *sessionHost, testConfig.Upn)
		assert.Nil(t, err)
	}
}

func _TestAssignSessionHost(t *testing.T) {
	err = initAVD()
	assert.Nil(t, err)

	err = avd.AssignSessionHost(ctx, testConfig.HostPool, testConfig.SessionHost, testConfig.Upn)
	assert.Nil(t, err)
}

// FIXME: use session host obj in delete action
// func _TestDeleteSessionHost(t *testing.T) {
// 	err = initAVD()
// 	assert.Nil(t, err)

// 	sessionHost = avd.GetSessionHost(ctx, testConfig.SessionHost)

// 	err = avd.DeleteSessionHost(ctx, sessionHost)
// 	assert.Nil(t, err)
// }

func _TestDeleteUserSession(t *testing.T) {
	err = initAVD()
	assert.Nil(t, err)

	err = avd.DeleteUserSession(ctx, testConfig.HostPool, testConfig.SessionHost, testConfig.Upn)
	assert.Nil(t, err)
}

func _TestDisconnectUserSession(t *testing.T) {
	err = initAVD()
	assert.Nil(t, err)

	err = avd.DisconnectUserSession(ctx, testConfig.HostPool, testConfig.SessionHost, testConfig.Upn)
	assert.Nil(t, err)
}

func _TestAssignUserToRoles(t *testing.T) {
	err = initAVD()
	assert.Nil(t, err)

	err = avd.AssignRoleToUser(ctx, testConfig.DesktopVirtualizationUserRoleId, testConfig.UserObjectId)
	assert.Nil(t, err)

	err = avd.AssignRoleToUser(ctx, testConfig.VirtualMachineUserLoginRoleId, testConfig.UserObjectId)
	assert.Nil(t, err)
}

func TestGetNextPhoneticName(t *testing.T) {
	tests := []struct {
		name         string
		current      string
		maxSequences int
		expected     string
		expectError  bool
	}{
		{
			name:         "Initial value (empty string)",
			current:      "",
			maxSequences: 3,
			expected:     "ALPHA",
			expectError:  true,
		},
		{
			name:         "Next phonetic after ALPHA",
			current:      "ALPHA",
			maxSequences: 3,
			expected:     "BRAVO",
			expectError:  false,
		},
		{
			name:         "Next phonetic after ZULU with room for more sequences",
			current:      "ZULU",
			maxSequences: 3,
			expected:     "ALPHA-ALPHA",
			expectError:  false,
		},
		{
			name:         "Next phonetic after ALPHA-ALPHA",
			current:      "ALPHA-ALPHA",
			maxSequences: 3,
			expected:     "ALPHA-BRAVO",
			expectError:  false,
		},
		{
			name:         "Exceeded maxSequences",
			current:      "ALPHA-ALPHA-ALPHA",
			maxSequences: 2,
			expected:     "",
			expectError:  true,
		},
		{
			name:         "Exceeded maxSequences",
			current:      "ZULU-ZULU",
			maxSequences: 3,
			expected:     "ALPHA-ALPHA-ALPHA",
			expectError:  false,
		},
		{
			name:         "Exceeded maxSequences",
			current:      "ZULU-ZULU",
			maxSequences: 2,
			expected:     "",
			expectError:  true,
		},
		{
			name:         "Invalid input starts over",
			current:      "INVALID",
			maxSequences: 3,
			expected:     "",
			expectError:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			startTime := time.Now()
			result, err := getNextPhoneticWord(tc.current, tc.maxSequences)
			elapsedTime := time.Since(startTime)

			fmt.Printf("Test '%s' took %s\n", tc.name, elapsedTime)

			if tc.expectError {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				assert.NoError(t, err, "Unexpected error: %v", err)
				assert.Equal(t, tc.expected, result, "Unexpected result for input: %s", tc.current)
			}
		})
	}
}

func TestGenerateNextName(t *testing.T) {
	tests := []struct {
		name         string
		existing     []string
		baseName     string
		maxSequences int
		expected     string
		expectError  bool
	}{
		{
			name:         "Initial name generation",
			existing:     []string{},
			maxSequences: 3,
			expected:     "ALPHA",
			expectError:  false,
		},
		{
			name:         "Next name generation after ALPHA",
			existing:     []string{"ALPHA"},
			maxSequences: 3,
			expected:     "BRAVO",
			expectError:  false,
		},
		{
			name:         "Next name generation with sequences",
			existing:     []string{"ZULU"},
			maxSequences: 3,
			expected:     "ALPHA-ALPHA",
			expectError:  false,
		},
		{
			name:         "Exceeds max sequences",
			existing:     []string{"ALPHA-ALPHA", "ALPHA-BRAVO"},
			maxSequences: 1,
			expected:     "",
			expectError:  true,
		},
		{
			name:         "Invalid names",
			existing:     []string{"INVALID"},
			maxSequences: 3,
			expected:     "",
			expectError:  true,
		},
		{
			name:         "Multiple existing names",
			existing:     []string{"ALPHA", "BRAVO", "CHARLIE"},
			maxSequences: 3,
			expected:     "DELTA",
			expectError:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			startTime := time.Now()
			var lastExisting string
			if len(tc.existing) > 0 {
				lastExisting = tc.existing[len(tc.existing)-1]
			}
			result, err := GenerateNextName(lastExisting, tc.maxSequences)
			elapsedTime := time.Since(startTime)

			fmt.Printf("Test '%s' took %s\n", tc.name, elapsedTime)

			if tc.expectError {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				assert.NoError(t, err, "Unexpected error: %v", err)
				assert.Equal(t, tc.expected, result, "Unexpected result")
			}
		})
	}
}
