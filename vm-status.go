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
	cfg.SubscriptionID, found = cloudy.MapKeyStr(config, "SubscriptionID", true)
	if !found {
		return nil, errors.New("SubscriptionID required")
	}
	cfg.ResourceGroup, found = cloudy.MapKeyStr(config, "ResourceGroup", true)
	if !found {
		return nil, errors.New("ResourceGroup required")
	}
	cfg.TenantID, found = cloudy.MapKeyStr(config, "TenantID", true)
	if !found {
		return nil, errors.New("TenantID required")
	}
	cfg.ClientID, found = cloudy.MapKeyStr(config, "ClientID", true)
	if !found {
		return nil, errors.New("ClientID required")
	}
	cfg.ClientSecret, found = cloudy.MapKeyStr(config, "ClientSecret", true)
	if !found {
		return nil, errors.New("ClientSecret required")
	}
	cfg.Region, found = cloudy.MapKeyStr(config, "Region", true)
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
	return &AzureVMController{
		Client: client,
		Config: config,
	}, nil
}

func (vmc *AzureVMController) ListAll(ctx context.Context) ([]*cloudyvm.VirtualMachineStatus, error) {
	return VmList(ctx, vmc.Client)
}

func (vmc *AzureVMController) ListWithTag(ctx context.Context, tag string) ([]*cloudyvm.VirtualMachineStatus, error) {
	return VmList(ctx, vmc.Client)
}

func (vmc *AzureVMController) Status(ctx context.Context, vmName string) (*cloudyvm.VirtualMachineStatus, error) {
	return VmStatus(ctx, vmc.Client, vmName)
}

func (vmc *AzureVMController) SetState(ctx context.Context, state cloudyvm.VirtualMachineAction, vmName string, wait bool) (*cloudyvm.VirtualMachineStatus, error) {
	return VmState(ctx, vmc.Client, state, vmName, wait)
}

func (vmc *AzureVMController) Start(ctx context.Context, vmName string, wait bool) error {
	return VmStart(ctx, vmc.Client, vmName, wait)
}

func (vmc *AzureVMController) Stop(ctx context.Context, vmName string, wait bool) error {
	return VmStop(ctx, vmc.Client, vmName, wait)
}

func (vmc *AzureVMController) Terminate(ctx context.Context, vmName string, wait bool) error {
	return VmTerminate(ctx, vmc.Client, vmName, wait)
}
