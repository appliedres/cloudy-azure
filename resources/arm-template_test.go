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
   "$schema":"https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
   "contentVersion":"1.0.0.0",
   "parameters":{
      "storageAccountName":{
         "type":"string",
         "minLength":3,
         "maxLength":24
      },
      "location":{
         "type":"string",
         "defaultValue":"eastus"
      },
      "sku":{
         "type":"string",
         "defaultValue":"Standard_LRS",
         "allowedValues":[
            "Standard_LRS",
            "Standard_GRS",
            "Standard_ZRS",
            "Premium_LRS"
         ]
      }
   },
   "resources":[
      {
         "type":"Microsoft.Storage/storageAccounts",
         "apiVersion":"2022-09-01",
         "name":"[parameters('storageAccountName')]",
         "location":"[parameters('location')]",
         "sku":{
            "name":"[parameters('sku')]"
         },
         "kind":"StorageV2",
         "properties":{
            
         }
      }
   ],
   "outputs":{
      "storageAccountEndpoint":{
         "type":"string",
         "value":"[reference(parameters('storageAccountName')).primaryEndpoints.blob]"
      }
   }
}`

	params := `{
   "$schema":"https://schema.management.azure.com/schemas/2019-04-01/deploymentParameters.json#",
   "contentVersion":"1.0.0.0",
   "parameters":{
      "storageAccountName":{
         "value":"teststorageacct01"
      },
      "location":{
         "value":"usgovvirginia"
      },
      "sku":{
         "value":"Standard_LRS"
      }
   }
}`

	err = arm.ExecuteArmTemplate(ctx, "TestCreateArm", []byte(armTemplate), []byte(params))
	assert.NoError(t, err)
}
