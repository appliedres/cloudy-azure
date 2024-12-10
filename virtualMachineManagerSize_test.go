package cloudyazure

// import (
// 	"testing"

// 	"github.com/appliedres/cloudy"
// 	"github.com/appliedres/cloudy/models"

// 	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
// 	"github.com/stretchr/testify/assert"
// )

// func TestFindBestVMSize(t *testing.T) {
// 	env := cloudy.CreateCompleteEnvironment("ARKLOUD_ENV", "", "")
// 	cloudy.SetDefaultEnvironment(env)

// 	creds := AzureCredentials{
// 		Region: env.Force("AZ_REGION", ""),
// 		TenantID: env.Force("AZ_TENANT_ID", ""),
// 		ClientID: env.Force("AZ_CLIENT_ID", ""),
// 		ClientSecret: env.Force("AZ_CLIENT_SECRET", ""),
// 		ResourceGroup: env.Force("AZ_RESOURCE_GROUP", ""),
// 		SubscriptionID: env.Force("AZ_SUBSCRIPTION_ID", ""),
// 	}

// 	config := VirtualMachineManagerConfig{
		
// 	}
		
// 	vmSizes := []*armcompute.VirtualMachineSize{
// 		{
// 			Name:                stringPtr("Standard_A2_v2"),
// 			NumberOfCores:       2,
// 			MemoryInMB:          4096,
// 			MaxNetworkInterfaces: 1,
// 			ResourceDiskSizeInMB: int64Ptr(10240),
// 		},
// 		{
// 			Name:                stringPtr("Standard_D4_v3"),
// 			NumberOfCores:       4,
// 			MemoryInMB:          16384,
// 			MaxNetworkInterfaces: 2,
// 			ResourceDiskSizeInMB: int64Ptr(10240),
// 		},
// 		{
// 			Name:                stringPtr("Standard_NC6"),
// 			NumberOfCores:       6,
// 			MemoryInMB:          57344,
// 			MaxNetworkInterfaces: 4,
// 			ResourceDiskSizeInMB: int64Ptr(102400),
// 		},
// 	}

// 	template := models.VirtualMachineTemplate{
// 		MinCPU: 4, MaxCPU: 8,
// 		MinRAM: 16, MaxRAM: 32,
// 		MinNic: 2, MaxNic: 4,
// 		MinGpu: 1, MaxGpu: 2,
// 	}

// 	vmm := NewAzureVirtualMachineManager(ctx, creds, config)

// 	bestSize, nearMatches := vmm.FindBestAzureVMSize(ctx, vmSizes, template)
// 	assert.Equal(t, "Standard_D4_v3", *bestSize.Name, "unexpected best size")
// 	assert.Contains(t, nearMatches, vmSizes[2], "expected NC6 to be a near match")
// }

// // Utility functions
// func stringPtr(s string) *string {
// 	return &s
// }

// func int64Ptr(i int64) *int64 {
// 	return &i
// }