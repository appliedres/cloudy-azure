package cloudyazure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/pkg/errors"
)

func (vmm *AzureVirtualMachineManager) RunCommand(ctx context.Context, vmId string, script string) error {

	poller, err := vmm.vmClient.BeginRunCommand(ctx, vmm.credentials.ResourceGroup, vmId, armcompute.RunCommandInput{
		CommandID: to.Ptr("GoRunPowerShell"),
		Script:    []*string{&script},
	}, nil)

	if err != nil {
		return errors.Wrap(err, "Run Command")
	}

	_, err = pollWrapper(ctx, poller, "Run Command")
	if err != nil {
		return errors.Wrap(err, "Run Command")
	}

	return nil
}
