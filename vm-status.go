package cloudyazure

import (
	"context"
	"errors"

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
}

type AzureVMController struct {
	Client *armcompute.VirtualMachinesClient
	Usage  *armcompute.UsageClient
	Config *AzureVMControllerConfig
}

type AzureVMControllerFactory struct{}

func (f *AzureVMControllerFactory) Create(cfg interface{}) (cloudyvm.VMController, error) {
	azCfg := cfg.(*AzureVMControllerConfig)
	if azCfg == nil {
		return nil, cloudy.ErrInvalidConfiguration
	}

	return NewAzureVMController(context.Background(), azCfg)
}

func (f *AzureVMControllerFactory) ToConfig(config map[string]interface{}) (interface{}, error) {
	var found bool

	cfg := &AzureVMControllerConfig{}
	cfg.SubscriptionID, found = cloudy.EnvKeyStr(config, "SubscriptionID")
	if !found {
		return nil, errors.New("SubscriptionID required")
	}
	cfg.ResourceGroup, found = cloudy.EnvKeyStr(config, "ResourceGroup")
	if !found {
		return nil, errors.New("ResourceGroup required")
	}
	cfg.TenantID, found = cloudy.EnvKeyStr(config, "TenantID")
	if !found {
		return nil, errors.New("TenantID required")
	}
	cfg.ClientID, found = cloudy.EnvKeyStr(config, "ClientID")
	if !found {
		return nil, errors.New("ClientID required")
	}
	cfg.ClientSecret, found = cloudy.EnvKeyStr(config, "ClientSecret")
	if !found {
		return nil, errors.New("ClientSecret required")
	}
	cfg.Region, found = cloudy.EnvKeyStr(config, "Region")
	if !found {
		return nil, errors.New("Region required")
	}

	return cfg, nil
}

func NewAzureVMController(ctx context.Context, config *AzureVMControllerConfig) (*AzureVMController, error) {
	client, err := NewVMClient(ctx, config)
	if err != nil {
		return nil, err
	}

	usage, err := NewUsageClient(ctx, config)
	if err != nil {
		return nil, err
	}

	return &AzureVMController{
		Client: client,
		Usage:  usage,
		Config: config,
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

func (vmc *AzureVMController) Create(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) (*cloudyvm.VirtualMachineConfiguration, error) {
	return VmCreate(ctx, vmc.Client, vm)
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
