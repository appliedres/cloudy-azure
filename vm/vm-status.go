package vm

import (
	"context"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/appliedres/cloudy"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy-azure/keyvault"
	cloudyvm "github.com/appliedres/cloudy/vm"
	"github.com/hashicorp/go-version"
)

const AzureArmCompute = "azure-arm-compute"

func init() {
	cloudyvm.VmControllers.Register(AzureArmCompute, &AzureVMControllerFactory{})
}

type AzureVMControllerConfig struct {
	cloudyazure.AzureCredentials
	SubscriptionID string
	ResourceGroup  string

	// ??
	NetworkResourceGroup            string // From Environment Variable
	SourceImageGalleryResourceGroup string
	SourceImageGalleryName          string   // From Environment Variable
	Vnet                            string   // From Environment Variable
	AvailableSubnets                []string // From Environment Variable
	NetworkSecurityGroupName        string   // From Environment Variable
	NetworkSecurityGroupID          string   // From Environment Variable
	// SaltCmd                         string   // From Environment Variable
	VaultURL string

	DomainControllerOverride string
	DomainControllers        []*string // From Environment Variable

	LogBody bool
}

type AzureVMController struct {
	Vault  *keyvault.KeyVault
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

func (f *AzureVMControllerFactory) FromEnv(env *cloudy.Environment) (interface{}, error) {
	cfg := &AzureVMControllerConfig{}
	cfg.AzureCredentials = cloudyazure.GetAzureCredentialsFromEnv(env)
	cfg.SubscriptionID = env.Force("AZ_SUBSCRIPTION_ID")
	cfg.ResourceGroup = env.Force("AZ_RESOURCE_GROUP")
	cfg.SubscriptionID = env.Force("AZ_SUBSCRIPTION_ID")

	// Not always necessary but needed for creation
	cfg.NetworkResourceGroup = env.Force("AZ_NETWORK_RESOURCE_GROUP")
	cfg.SourceImageGalleryResourceGroup = env.Default("AZ_SOURCE_IMAGE_GALLERY_RESOURCE_GROUP", cfg.ResourceGroup)
	cfg.SourceImageGalleryName = env.Force("AZ_SOURCE_IMAGE_GALLERY_NAME")
	cfg.Vnet = env.Force("AZ_VNET")
	cfg.NetworkSecurityGroupName = env.Force("AZ_NETWORK_SECURITY_GROUP_NAME")
	cfg.NetworkSecurityGroupID = env.Force("AZ_NETWORK_SECURITY_GROUP_ID")
	cfg.VaultURL = env.Force("AZ_VAULT_URL")

	subnets := env.Force("SUBNETS") //SUBNET1,SUBNET2
	cfg.AvailableSubnets = strings.Split(subnets, ",")

	// Defaults to true for backwards compatibility
	cfg.DomainControllerOverride = env.Default("DOMAIN_CONTROLLER_OVERRIDE", "True")
	domainControllers := strings.Split(env.Force("DOMAIN_CONTROLLERS"), ",") // DC1, DC2
	for i := range domainControllers {
		cfg.DomainControllers = append(cfg.DomainControllers, &domainControllers[i])
	}

	logBody := env.Get("AZ_LOG_BODY")
	if strings.ToUpper(logBody) == "TRUE" {
		cfg.LogBody = true
	}

	return cfg, nil
}

func NewAzureVMController(ctx context.Context, config *AzureVMControllerConfig) (*AzureVMController, error) {
	cred, err := cloudyazure.GetAzureClientSecretCredential(config.AzureCredentials)
	if err != nil {
		return nil, cloudy.Error(ctx, "Authentication failure: %+v", err)
	}

	// client, err := NewVMClient(ctx, config)
	// if err != nil {
	// 	return nil, err
	// }

	// usage, err := NewUsageClient(ctx, config)
	// if err != nil {
	// 	return nil, err
	// }

	v, err := keyvault.NewKeyVault(ctx, config.VaultURL, config.AzureCredentials)
	if err != nil {
		return nil, err
	}

	return &AzureVMController{
		// Client: client,
		// Usage:  usage,
		Config: config,
		cred:   cred,
		Vault:  v,
	}, nil
}

func (vmc *AzureVMController) ListAll(ctx context.Context) ([]*cloudyvm.VirtualMachineStatus, error) {
	// return VmList(ctx, vmc.Client, vmc.Config.ResourceGroup)
	return nil, nil
}

func (vmc *AzureVMController) ListWithTag(ctx context.Context, tag string) ([]*cloudyvm.VirtualMachineStatus, error) {
	// return VmList(ctx, vmc.Client, vmc.Config.ResourceGroup)
	return nil, nil
}

func (vmc *AzureVMController) Status(ctx context.Context, vmName string) (*cloudyvm.VirtualMachineStatus, error) {
	// return VmStatus(ctx, vmc.Client, vmName, vmc.Config.ResourceGroup)
	return nil, nil
}

func (vmc *AzureVMController) SetState(ctx context.Context, state cloudyvm.VirtualMachineAction, vmName string, wait bool) (*cloudyvm.VirtualMachineStatus, error) {
	// return VmState(ctx, vmc.Client, state, vmName, vmc.Config.ResourceGroup, wait)
	return nil, nil
}

func (vmc *AzureVMController) Start(ctx context.Context, vmName string, wait bool) error {
	// return VmStart(ctx, vmc.Client, vmName, vmc.Config.ResourceGroup, wait)
	return nil
}

func (vmc *AzureVMController) Stop(ctx context.Context, vmName string, wait bool) error {
	// return VmStop(ctx, vmc.Client, vmName, vmc.Config.ResourceGroup, wait)
	return nil
}

func (vmc *AzureVMController) Terminate(ctx context.Context, vmName string, wait bool) error {
	// return VmTerminate(ctx, vmc.Client, vmName, vmc.Config.ResourceGroup, wait)
	return nil
}

func (vmc *AzureVMController) GetLimits(ctx context.Context) ([]*cloudyvm.VirtualMachineLimit, error) {

	pager := vmc.Usage.NewListPager(vmc.Config.Region, &armcompute.UsageClientListOptions{})

	var rtn []*cloudyvm.VirtualMachineLimit

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, cloudy.Error(ctx, "Error retrieving next page: %v", err)
		}

		for _, u := range resp.Value {
			lowerName := strings.ToLower(*u.Name.Value)

			// Eliminate non-VM values
			if strings.Contains(lowerName, "family") && !strings.Contains(lowerName, "promo") {
				rtn = append(rtn, &cloudyvm.VirtualMachineLimit{
					Name:    *u.Name.Value,
					Current: int(*u.CurrentValue),
					Limit:   int(*u.Limit),
				})
			}
		}
	}

	return rtn, nil
}

func (vmc *AzureVMController) GetLatestImageVersion(ctx context.Context, imageName string) (string, error) {
	c, err := armcompute.NewSharedGalleryImageVersionsClient(vmc.Config.SubscriptionID, vmc.cred, nil)
	if err != nil {
		return "", err
	}
	pager := c.NewListPager(vmc.Config.Region, vmc.Config.SourceImageGalleryName, imageName, &armcompute.SharedGalleryImageVersionsClientListOptions{})

	var allVersions []*version.Version

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return "", err
		}
		for _, imageVersion := range resp.Value {
			v, err := version.NewVersion(*imageVersion.Name)
			if err != nil {
				_ = cloudy.Error(ctx, "Skipping Invalid Version : %v, %v", *imageVersion.Name, err)
				continue
			}
			allVersions = append(allVersions, v)
		}
	}

	sort.Sort(version.Collection(allVersions))

	latest := allVersions[len(allVersions)-1]

	return latest.Original(), nil
}

func (vmc *AzureVMController) GetVMSizes(ctx context.Context) (map[string]*cloudyvm.VmSize, error) {
	cloudy.Info(ctx, "AzureVMController.GetVMSizes")

	client, err := armcompute.NewResourceSKUsClient(vmc.Config.SubscriptionID, vmc.cred, &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	})
	if err != nil {
		return nil, cloudy.Error(ctx, "AzureVMController.GetVMSizes could not create NewResourceSKUsClient, %v", err)
	}

	sizes := make(map[string]*cloudyvm.VmSize)
	pager := client.NewListPager(&armcompute.ResourceSKUsClientListOptions{
		Filter:                   to.Ptr("location eq " + vmc.Config.Region),
		IncludeExtendedLocations: nil,
	})
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return sizes, cloudy.Error(ctx, "AzureVMController.GetVMSizes could not get NextPage, %v", err)
		}

		for _, r := range resp.Value {
			if strings.EqualFold("virtualMachines", *r.ResourceType) &&
				!strings.Contains(*r.Size, "Promo") &&
				IsInLocation(vmc.Config.Region, r.Locations) &&
				IsAvailable(r.Restrictions) {

				size := SizeFromResource(ctx, r)
				sizes[size.Name] = size
			}
		}
	}

	cloudy.Info(ctx, "AzureVMController.GetVMSizes %d sizes found", len(sizes))

	return sizes, nil
}

func IsInLocation(region string, locations []*string) bool {
	for _, location := range locations {
		if strings.EqualFold(region, *location) {
			return true
		}
	}

	return false

}

func IsAvailable(restrictions []*armcompute.ResourceSKURestrictions) bool {

	notAvailable := string(armcompute.ResourceSKURestrictionsReasonCodeNotAvailableForSubscription)

	for _, restriction := range restrictions {

		if strings.EqualFold(notAvailable, string(*restriction.ReasonCode)) {
			return false
		}
	}

	return true
}
