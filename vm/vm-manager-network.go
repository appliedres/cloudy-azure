package vm

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v5"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	"github.com/pkg/errors"
)

func (vmm *AzureVirtualMachineManager) GetAllNics(ctx context.Context) ([]*models.VirtualMachineNic, error) {
	nics := []*models.VirtualMachineNic{}

	pager := vmm.nicClient.NewListAllPager(&armnetwork.InterfacesClientListAllOptions{})

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, v := range resp.Value {
			nic := models.VirtualMachineNic{
				ID:        *v.ID,
				Name:      *v.Name,
				PrivateIP: *v.Properties.IPConfigurations[0].Properties.PrivateIPAddress,
			}

			nics = append(nics, &nic)

		}
	}

	return nics, nil
}

func (vmm *AzureVirtualMachineManager) GetNics(ctx context.Context, vmId string) ([]*models.VirtualMachineNic, error) {
	nics := []*models.VirtualMachineNic{}

	allNics, err := vmm.GetAllNics(ctx)
	if err != nil {
		return nil, err
	}

	for _, nic := range allNics {
		// Match by name
		if strings.Contains(nic.Name, vmId) {
			nics = append(nics, nic)
		}
	}

	return nics, nil
}
func (vmm *AzureVirtualMachineManager) CreateNic(ctx context.Context, vm *models.VirtualMachine) (*models.VirtualMachineNic, error) {
	log := logging.GetLogger(ctx)

	subnetId, err := vmm.findBestSubnet(ctx)
	if err != nil {
		return nil, err
	}

	nicName := fmt.Sprintf("%s-nic-primary", vm.ID)

	fullSubnetId := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s/subnets/%s",
		vmm.Credentials.SubscriptionID, vmm.Config.VnetResourceGroup, vmm.Config.VnetId, subnetId)

	dnsServers := []*string{}
	if strings.EqualFold(vm.Template.OperatingSystem, models.VirtualMachineTemplateOperatingSystemWindows) {
		dnsServers = vmm.Config.DomainControllers
	}
	// TODO: linux dns servers

	poller, err := vmm.nicClient.BeginCreateOrUpdate(ctx, vmm.Config.VnetResourceGroup, nicName, armnetwork.Interface{
		Location: &vmm.Credentials.Region,
		Tags:     generateAzureTagsForVM(vm),
		Properties: &armnetwork.InterfacePropertiesFormat{
			EnableAcceleratedNetworking: vm.Template.AcceleratedNetworking,
			IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
				{
					Name: to.Ptr(fmt.Sprintf("%v-ip", vm.ID)),
					Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
						Subnet: &armnetwork.Subnet{
							ID:         &fullSubnetId,
							Properties: &armnetwork.SubnetPropertiesFormat{},
						},
						PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
					},
				},
			},
			DNSSettings: &armnetwork.InterfaceDNSSettings{
				DNSServers: dnsServers,
			},
		},
	}, nil)
	if err != nil {
		return nil, err
	}

	resp, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}

	// Return the IP and NIC ID
	nic := &models.VirtualMachineNic{
		ID:        *resp.ID,
		Name:      *resp.Name,
		PrivateIP: *resp.Interface.Properties.IPConfigurations[0].Properties.PrivateIPAddress,
		// FIXME: add nic resource group to model
	}

	log.InfoContext(ctx, fmt.Sprintf("Created new NIC: %s", nic.ID))

	return nic, nil
}

func (vmm *AzureVirtualMachineManager) DeleteNics(ctx context.Context, nics []*models.VirtualMachineNic) error {

	for _, nic := range nics {
		err := vmm.DeleteNic(ctx, nic)
		if err != nil {
			return errors.Wrap(err, "DeleteNics")
		}
	}

	return nil
}

func (vmm *AzureVirtualMachineManager) DeleteNic(ctx context.Context, nic *models.VirtualMachineNic) error {
	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, fmt.Sprintf("DeleteNic starting: %s", nic.Name))

	poller, err := vmm.nicClient.BeginDelete(ctx, vmm.Config.VnetResourceGroup, nic.Name, nil)
	if err != nil {
		return errors.Wrap(err, "DeleteNic.BeginDelete")
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "DeleteNic.PollUntilDone")
	}

	return nil
}

func (vmm *AzureVirtualMachineManager) findBestSubnet(ctx context.Context) (string, error) {
	log := logging.GetLogger(ctx)

	bestSubnetId := ""
	bestSubnetCount := 0

	for _, subnetId := range vmm.Config.SubnetIds {
		subnetCount, err := vmm.getSubnetAvailableIps(ctx, subnetId)
		if err != nil {
			log.ErrorContext(ctx, fmt.Sprintf("error counting ips in subnet: %s", subnetId), logging.WithError(err))
			continue
		}

		if bestSubnetId == "" || subnetCount > bestSubnetCount {
			bestSubnetId = subnetId
			bestSubnetCount = subnetCount
		}
	}

	if bestSubnetId == "" {
		return "", fmt.Errorf("could not find any suitable subnets")
	}

	return bestSubnetId, nil
}

func (vmm *AzureVirtualMachineManager) getSubnetAvailableIps(ctx context.Context, subnetId string) (int, error) {
	res, err := vmm.subnetClient.Get(ctx,
		vmm.Config.VnetResourceGroup,
		vmm.Config.VnetId,
		subnetId,
		&armnetwork.SubnetsClientGetOptions{Expand: nil})
	if err != nil {
		return 0, errors.Wrap(err, "getSubnetAvailableIps")
	}

	var addressPrefix *string

	if len(res.Subnet.Properties.AddressPrefixes) > 0 {
		addressPrefix = res.Subnet.Properties.AddressPrefixes[0]
	} else if res.Subnet.Properties.AddressPrefix != nil {
		addressPrefix = res.Subnet.Properties.AddressPrefix
	}

	if addressPrefix == nil {
		return 0, fmt.Errorf("getSubnetAvailableIps - addressprefix not found")
	}

	maskParts := strings.Split(*addressPrefix, "/")
	if len(maskParts) != 2 {
		return 0, fmt.Errorf("getSubnetAvailableIps - invalid address prefix: %s", *addressPrefix)
	}

	subnetMask, err := strconv.Atoi(maskParts[1])
	if err != nil {
		return 0, errors.Wrapf(err, "getSubnetAvailableIps - invalid subnet mask %s", maskParts[1])
	}

	netmaskLength := int(math.Pow(2, float64(32-subnetMask)))

	// Azure reserves 5 IP addresses per subnet
	availableIPs := netmaskLength - 5 - len(res.Subnet.Properties.IPConfigurations)

	return availableIPs, nil
}
