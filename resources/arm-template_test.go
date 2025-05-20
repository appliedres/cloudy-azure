package resources

import (
	"os"
	"testing"

	"github.com/appliedres/cloudy"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/stretchr/testify/assert"
)

func InitArm() (*ArmManager, error) {
	err := cloudy.LoadEnv("../.env.local")
	if err != nil {
		return nil, err
	}

	ctx := cloudy.StartContext()

	clientId := os.Getenv("AZ_CLIENT_ID")
	clientSecret := os.Getenv("AZ_CLIENT_SECRET")
	tenantId := os.Getenv("AZ_TENANT_ID")
	subscriptionId := os.Getenv("AZ_SUBSCRIPTION_ID")
	resourceGroup := os.Getenv("DEFAULT_RG")

	creds := cloudyazure.AzureCredentials{
		TenantID:       tenantId,
		ClientID:       clientId,
		ClientSecret:   clientSecret,
		Region:         "usgovvirginia",
		SubscriptionID: subscriptionId,
	}

	config := ArmConfig{
		PollingTimeoutDuration: "30",
		ResourceGroup:          resourceGroup,
	}

	arm, err := NewArmManager(ctx, &config, &creds)
	if err != nil {
		return nil, err
	}
	return arm, err
}

func TestCreateArm(t *testing.T) {
	arm, err := InitArm()
	if err != nil {
		t.Fatal(err)
	}
	assert.NotNil(t, arm)
	ctx := cloudy.StartContext()

	armTemplate := `{
  "$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
  "contentVersion": "1.0.0.0",
  "parameters": {
    "storageAccountName": {
      "type": "string",
      "defaultValue": "simplestorageacct123",
      "minLength": 3,
      "maxLength": 24
    }
  },
  "resources": [
    {
      "type": "Microsoft.Storage/storageAccounts",
      "apiVersion": "2019-06-01",
      "name": "[parameters('storageAccountName')]",
      "location": "usgovvirginia",
      "sku": {
        "name": "Standard_LRS"
      },
      "kind": "StorageV2",
      "properties": {}
    }
  ]
}
`

	params := `{
  "$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentParameters.json#",
  "contentVersion": "1.0.0.0",
  "parameters": {
    "storageAccountName": {
      "value": "simplestorageacct123"
    }
  }
}
`
	var template map[string]interface{}
	var paramsObj map[string]interface{}

	template, paramsObj, err = arm.ValidateArmTemplate(ctx, "TestCreateArm", []byte(armTemplate), []byte(params))
	assert.NoError(t, err)

	err = arm.Deploy(ctx, "TestCreateArm", template, paramsObj)
	assert.NoError(t, err)
}
