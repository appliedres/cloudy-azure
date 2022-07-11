package cloudyazure

import (
	"context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/appliedres/cloudy"
	cloudyvm "github.com/appliedres/cloudy/vm"
)

const AzureArmCompute = "azure-arm-compute"

func init() {
	cloudyvm.VmControllers.Register(AzureArmCompute, &AzureVMControllerFactory{})
}

type AzureVMControllerConfig struct {
	AzureCredentials
	SubscriptionID string
	ResourceGroup  string

	// ??
	NetworkResourceGroup     string   // From Environment Variable
	SourceImageGalleryName   string   // From Environment Variable
	Vnet                     string   // From Environment Variable
	AvailableSubnets         []string // From Environment Variable
	NetworkSecurityGroupName string   // From Environment Variable
	NetworkSecurityGroupID   string   // From Environment Variable
	SaltCmd                  string
	WindowsSaltFileBase64    string
	VaultURL                 string
}

type AzureVMController struct {
	Vault  *KeyVault
	Client *armcompute.VirtualMachinesClient
	Usage  *armcompute.UsageClient
	Config *AzureVMControllerConfig
	cred   *azidentity.ClientSecretCredential
}

type AzureVMControllerFactory struct{}

func (f *AzureVMControllerFactory) Create(cfg interface{}) (cloudyvm.VMController, error) {
	azCfg := cfg.(*AzureVMControllerConfig)
	if azCfg == nil {
		return nil, cloudy.ErrInvalidConfiguration
	}

	return NewAzureVMController(context.Background(), azCfg)
}

func (f *AzureVMControllerFactory) FromEnv(env *cloudy.SegmentedEnvironment) (interface{}, error) {
	cfg := &AzureVMControllerConfig{}
	cfg.AzureCredentials = GetAzureCredentialsFromEnv(env)
	cfg.SubscriptionID = env.Force("AZ_SUBSCRIPTION_ID")
	cfg.ResourceGroup = env.Force("AZ_RESOURCE_GROUP")
	cfg.SubscriptionID = env.Force("AZ_SUBSCRIPTION_ID")

	// Not always necessary but needed for creation
	cfg.NetworkResourceGroup = env.Force("AZ_NETWORK_RESOURCE_GROUP")
	cfg.SourceImageGalleryName = env.Force("AZ_SOURCE_IMAGE_GALLERY_NAME")
	cfg.Vnet = env.Force("AZ_VNET")
	cfg.NetworkSecurityGroupName = env.Force("AZ_NETWORK_SECURITY_GROUP_NAME")
	cfg.NetworkSecurityGroupID = env.Force("AZ_NETWORK_SECURITY_GROUP_ID")
	cfg.VaultURL = env.Force("AZ_VAULT_URL")

	subnets := env.Force("SUBNETS") //SUBNET1,SUBNET2
	cfg.AvailableSubnets = strings.Split(subnets, ",")

	return cfg, nil
}

func NewAzureVMController(ctx context.Context, config *AzureVMControllerConfig) (*AzureVMController, error) {
	cred, err := GetAzureCredentials(config.AzureCredentials)
	if err != nil {
		return nil, cloudy.Error(ctx, "Authentication failure: %+v", err)
	}

	client, err := NewVMClient(ctx, config)
	if err != nil {
		return nil, err
	}

	usage, err := NewUsageClient(ctx, config)
	if err != nil {
		return nil, err
	}

	v, err := NewKeyVault(ctx, config.VaultURL, config.AzureCredentials)
	if err != nil {
		return nil, err
	}

	return &AzureVMController{
		Client: client,
		Usage:  usage,
		Config: config,
		cred:   cred,
		Vault:  v,
	}, nil
}

func (vmc *AzureVMController) ListAll(ctx context.Context) ([]*cloudyvm.VirtualMachineStatus, error) {
	return VmList(ctx, vmc.Client, vmc.Config.ResourceGroup)
}

func (vmc *AzureVMController) ListWithTag(ctx context.Context, tag string) ([]*cloudyvm.VirtualMachineStatus, error) {
	return VmList(ctx, vmc.Client, vmc.Config.ResourceGroup)
}

func (vmc *AzureVMController) Status(ctx context.Context, vmName string) (*cloudyvm.VirtualMachineStatus, error) {
	return VmStatus(ctx, vmc.Client, vmName, vmc.Config.ResourceGroup)
}

func (vmc *AzureVMController) SetState(ctx context.Context, state cloudyvm.VirtualMachineAction, vmName string, wait bool) (*cloudyvm.VirtualMachineStatus, error) {
	return VmState(ctx, vmc.Client, state, vmName, vmc.Config.ResourceGroup, wait)
}

func (vmc *AzureVMController) Start(ctx context.Context, vmName string, wait bool) error {
	return VmStart(ctx, vmc.Client, vmName, vmc.Config.ResourceGroup, wait)
}

func (vmc *AzureVMController) Stop(ctx context.Context, vmName string, wait bool) error {
	return VmStop(ctx, vmc.Client, vmName, vmc.Config.ResourceGroup, wait)
}

func (vmc *AzureVMController) Terminate(ctx context.Context, vmName string, wait bool) error {
	return VmTerminate(ctx, vmc.Client, vmc.Config.ResourceGroup, vmName, wait)
}

func (vmc *AzureVMController) GetLimits(ctx context.Context) ([]*cloudyvm.VirtualMachineLimit, error) {
	// pager := vmc.Usage.NewListPager()
	pager := vmc.Usage.NewListPager("", &armcompute.UsageClientListOptions{})

	var rtn []*cloudyvm.VirtualMachineLimit

	for {
		if !pager.More() {
			break
		}
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, u := range resp.Value {
			rtn = append(rtn, &cloudyvm.VirtualMachineLimit{
				Name:    *u.Name.Value,
				Current: int(*u.CurrentValue),
				Limit:   int(*u.Limit),
			})
		}
	}

	return rtn, nil
}

// func (vmc *AzureVMController) CheckQuota(ctx context.Context, vm *models.VirtualMachine) (bool, error) {

// 	resourceName := ""
// 	resourceScope := "subscriptions/00000000-0000-0000-0000-000000000000/providers/Microsoft.Compute/locations/eastus"

// 	quotaClient, err := armquota.NewClient(vmc.Config.AzureCredentials, &arm.ClientOptions{
// 		ClientOptions: policy.ClientOptions{
// 			Cloud: cloud.AzureGovernment,
// 		},
// 	})
// 	if err != nil {
// 		return false, err
// 	}

// 	quotaClientResponse, err := quotaClient.Get(ctx, resourceName, resourceScope, nil)
// 	if err != nil {
// 		// log.Fatalf("failed to finish the request: %v", err)
// 		return false, err
// 	}

// 	// TODO: Figure out how to get the actual quota value from the JSON object
// 	// https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/quota/armquota#LimitJSONObjectClassification
// 	quotaValue := quotaClientResponse.CurrentQuotaLimitBase.Properties.Limit.GetLimitJSONObject()
// 	quotaValue.GetLimitJSONObject()

// 	usagesClient, err := armquota.NewUsagesClient(cred, &arm.ClientOptions{
// 		ClientOptions: policy.ClientOptions{
// 			Cloud: cloud.AzureGovernment,
// 		},
// 	})
// 	if err != nil {
// 		return false, err
// 	}

// 	usagesClientResponse, err := usagesClient.Get(ctx, resourceName, resourceScope, nil)
// 	if err != nil {
// 		// log.Fatalf("failed to finish the request: %v", err)
// 		return false, err
// 	}

// 	usageValue := usagesClientResponse.CurrentUsagesBase.Properties.Usages.Value

// 	// TODO: Remove hard coded value
// 	if *usageValue < 100 {
// 		return true, nil
// 	}

// 	return false, nil
// }
