package vm

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/pkg/errors"
)

func (vmm *AzureVirtualMachineManager) RunCommand(ctx context.Context, vmId string, script string) error {

	poller, err := vmm.vmClient.BeginRunCommand(ctx, vmm.Credentials.ResourceGroup, vmId, armcompute.RunCommandInput{
		CommandID: to.Ptr("GoRunPowerShell"),
		Script:    []*string{&script},
	}, nil)

	if err != nil {
		return errors.Wrap(err, "Run Command")
	}

	_, err = cloudyazure.PollWrapper(ctx, poller, "Run Command")
	if err != nil {
		return errors.Wrap(err, "Run Command")
	}

	return nil
}
