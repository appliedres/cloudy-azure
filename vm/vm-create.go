package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v5"
	"github.com/appliedres/cloudy"
	cloudyazure "github.com/appliedres/cloudy-azure"
	cloudyvm "github.com/appliedres/cloudy/vm"
)

func (vmc *AzureVMController) Create(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) (*cloudyvm.VirtualMachineConfiguration, error) {
	cloudy.Info(ctx, "[%s] Starting ValidateConfiguration", vm.ID)
	err := vmc.ValidateConfiguration(ctx, vm)
	if err != nil {
		return vm, err
	}

	// Check if NIC already exists
	cloudy.Info(ctx, "[%s] Starting GetNIC", vm.ID)
	network, err := vmc.GetNIC(ctx, vm)
	if err != nil {
		cloudy.Info(ctx, "[%s] Error looking for NIC: %s", vm.ID, err.Error())
	}

	if network != nil {
		cloudy.Info(ctx, "[%s] Using existing NIC: %s", vm.ID, network.ID)
		vm.PrimaryNetwork = network
	} else {
		// Check / Create the Network Security Group
		cloudy.Info(ctx, "[%s] Starting FindBestSubnet: %s", vm.ID, vmc.Config.AvailableSubnets)
		subnetId, err := vmc.FindBestSubnet(ctx, vmc.Config.AvailableSubnets)
		if err != nil {
			return vm, err
		}
		if subnetId == "" {
			return vm, fmt.Errorf("[%s] no available subnets", vm.ID)
		}

		// Check / Create the Network Interface
		cloudy.Info(ctx, "[%s] Starting CreateNIC", vm.ID)
		err = vmc.CreateNIC(ctx, vm, subnetId)
		if err != nil {
			return vm, err
		}
	}

	cloudy.Info(ctx, "[%s] Starting CreateVirtualMachine", vm.ID)
	err = vmc.CreateVirtualMachine(ctx, vm)
	if err != nil {
		_ = cloudy.Error(ctx, "[%s] CreateVirtualMachine err: %s", vm.ID, err.Error())
	}
	return vm, err
}

func (vmc *AzureVMController) ValidateConfiguration(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) error {
	if strings.Contains(strings.ToLower(vm.OSType), "linux") {
	} else if strings.EqualFold(vm.OSType, "windows") {
	} else {
		return cloudy.Error(ctx, "[%s] invalid OS Type: %v, cannot create vm", vm.ID, vm.OSType)
	}

	return nil
}

// Finds the best subnet based on IP availabilty
func (vmc *AzureVMController) FindBestSubnet(ctx context.Context, availableSubnets []string) (string, error) {
	for _, subnet := range availableSubnets {
		available, err := vmc.GetAvailableIPS(ctx, subnet)

		if err != nil {
			return "", err
		}
		cloudy.Info(ctx, "Available IPs for subnet %s: %d", subnet, available)

		if available > 0 {
			return subnet, nil
		}
	}

	// No available subnets
	return "", nil
}

// Finds the best subnet based on IP availabilty
func (vmc *AzureVMController) GetAvailableIPS(ctx context.Context, subnet string) (int, error) {
	client, err := armnetwork.NewSubnetsClient(vmc.Config.SubscriptionID, vmc.cred, &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	})
	if err != nil {
		return -1, cloudy.Error(ctx, "failed to create client: %v", err)
	}

	res, err := client.Get(ctx,
		vmc.Config.NetworkResourceGroup,
		vmc.Config.Vnet,
		subnet,
		&armnetwork.SubnetsClientGetOptions{Expand: nil})
	if err != nil {
		return -1, cloudy.Error(ctx, "failed to finish the request: %v", err)
	}

	// Retrieve and parse the CIDR block
	addressPrefix := res.Subnet.Properties.AddressPrefix
	maskParts := strings.Split(*addressPrefix, "/")
	if len(maskParts) != 2 {
		return -1, cloudy.Error(ctx, "invalid address previx: %v", addressPrefix)
	}

	subnetMask, err := strconv.Atoi(maskParts[1])
	if err != nil {
		return -1, cloudy.Error(ctx, "invalid number in subnet mask: %v, %v", maskParts[1], err)
	}

	netmaskLength := int(math.Pow(2, float64(32-subnetMask)))

	// Azure reserves 5 IP adresses per subnet
	availableIPs := netmaskLength - 5 - len(res.Subnet.Properties.IPConfigurations)

	return availableIPs, err
}

/*
	data "azurerm_network_security_group" "vdi-security-group" {
	    name = var.vdi-nsg
	    resource_group_name = data.azurerm_resource_group.main-rg.name
	}
*/
func (vmc *AzureVMController) CreateNSG(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) (string, error) {
	// Create the appropriate client
	client, err := armnetwork.NewSecurityGroupsClient(vmc.Config.SubscriptionID, vmc.cred, nil)
	if err != nil {
		return "", cloudy.Error(ctx, "cloud not create the network security group client. Configuration error, %v", err)
	}

	//TODO: Double check that azure will not create 2 of these with the same name. If so then we
	//      need to add a GET() check in here first
	poller, err := client.BeginCreateOrUpdate(
		ctx,
		vmc.Config.ResourceGroup,
		vmc.Config.NetworkSecurityGroupName,
		armnetwork.SecurityGroup{
			Location: to.Ptr(vmc.Config.Region),
		},
		nil)

	if err != nil {
		log.Fatalf("failed to finish the request: %v", err)
		return "", cloudy.Error(ctx, "Failed generateing NSG, %v", err)
	}
	res, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return "", cloudy.Error(ctx, "failed to pull the result, %v", err)
	}

	return *res.SecurityGroup.ID, nil
}

func (vmc *AzureVMController) GetNSG(ctx context.Context, name string) (*armnetwork.SecurityGroup, error) {
	client, err := armnetwork.NewSecurityGroupsClient(vmc.Config.SubscriptionID, vmc.cred, &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	})
	if err != nil {
		return nil, cloudy.Error(ctx, "could not create the network security group client. Configuration error, %v", err)
	}

	resp, err := client.Get(ctx, vmc.Config.NetworkResourceGroup, name, nil)
	if err != nil {
		return nil, err
	}
	return &resp.SecurityGroup, err
}

/*
CreateNIC - Creates the Network Interface for the virtual machine. It mimics the terraform code listed below.
The elements used by this method are:
- VM Name / ID 		(from vm)
- Region			(from factory)
- Resource Group	(from factory)
- Subnet ID			(from vm)

Once created the NIC has an ID and an IP address that we care about. The VirtualMachineConfiguration input is
mutated to add the appropriate information.

 az network nic create \
 	--resource-group go-on-azure  \
	--vnet-name go-on-azure-vmVNET \
	--subnet go-on-azure-vmSubnet \
	--name uvm-gotest-ip

resource "azurerm_network_interface" "main-nic" {
    name                      = join("-", [var.vdi-name, random_string.random.result])
    location                  = data.azurerm_resource_group.main-rg.location
    resource_group_name       = data.azurerm_resource_group.main-rg.name

	ip_configuration {
		name                          = join("-", [var.vdi-name, "IP"])
		subnet_id                     = data.azurerm_subnet.main-subnet.id
		private_ip_address_allocation = "Dynamic"
    }
}*/
//NOT WORKING YET
func (vmc *AzureVMController) CreateNIC(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration, subnetId string) error {
	// Not sure why a random nic is created
	// random := cloudy.GenerateRandom(6)
	// nicName := fmt.Sprintf("%v-%v", vm.ID, random)
	nicName := fmt.Sprintf("%v-nic-primary", vm.ID)
	region := vmc.Config.Region
	rg := vmc.Config.NetworkResourceGroup

	if vm.Size == nil {
		return cloudy.Error(ctx, "[%s] Invalid VM Size %v", vm.ID, vm.Size)
	}
	acceleratedNetworking := vm.Size.AcceleratedNetworking

	nicClient, err := armnetwork.NewInterfacesClient(vmc.Config.SubscriptionID, vmc.cred, &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	})
	if err != nil {
		return err
	}

	fullSubId := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s/subnets/%s", vmc.Config.SubscriptionID, vmc.Config.NetworkResourceGroup, vmc.Config.Vnet, subnetId)

	dnsServers := []*string{}

	if strings.EqualFold(vmc.Config.DomainControllerOverride, "True") {
		if strings.EqualFold(vm.OSType, "windows") {
			dnsServers = vmc.Config.DomainControllers
		}
	}

	poller, err := nicClient.BeginCreateOrUpdate(ctx, rg, nicName, armnetwork.Interface{
		Location: &region,

		Properties: &armnetwork.InterfacePropertiesFormat{
			EnableAcceleratedNetworking: to.Ptr(acceleratedNetworking),
			IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
				{
					Name: to.Ptr(fmt.Sprintf("%v-ip", vm.ID)),
					Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
						Subnet: &armnetwork.Subnet{
							ID: &fullSubId,
							// Name: to.Ptr(subnetId),
							Properties: &armnetwork.SubnetPropertiesFormat{
								// NetworkSecurityGroup: nsg,
								// NetworkSecurityGroup: &armnetwork.SecurityGroup{
								// 	ID: nsg.ID,
								// },
							},
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
		return err
	}

	resp, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	// Store the IP and NIC ID
	vm.PrimaryNetwork = &cloudyvm.VirtualMachineNetwork{
		ID:        *resp.ID,
		Name:      *resp.Name,
		PrivateIP: *resp.Interface.Properties.IPConfigurations[0].Properties.PrivateIPAddress,
	}
	return nil
}

func (vmc *AzureVMController) GetVmOsDisk(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) (*cloudyvm.VirtualMachineDisk, error) {
	cloudy.Info(ctx, "[%s] Starting GetVmOsDisk Subscription: %s", vm.ID, vmc.Config.SubscriptionID)
	diskClient, err := armcompute.NewDisksClient(vmc.Config.SubscriptionID, vmc.cred,
		&arm.ClientOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloud.AzureGovernment,
			},
		})
	if err != nil {
		_ = cloudy.Error(ctx, "[%s] failed to create disks client: %v", vm.ID, err)
		return nil, err
	}

	pager := diskClient.NewListPager(&armcompute.DisksClientListOptions{})
	for pager.More() {
		nextResult, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, disk := range nextResult.Value {
			if disk.Name != nil && strings.Contains(*disk.Name, vm.ID) {
				vmDisk := cloudyvm.VirtualMachineDisk{
					Name: *disk.Name,
				}

				return &vmDisk, nil
			}
		}
	}

	return nil, nil
}

// Find VM if it already exists
func (vmc *AzureVMController) GetVM(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) (*cloudyvm.VirtualMachineConfiguration, error) {
	vmClient, err := armcompute.NewVirtualMachinesClient(vmc.Config.SubscriptionID, vmc.cred, &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	})
	if err != nil {
		return nil, err
	}

	vmResponse, err := vmClient.Get(ctx,
		vmc.Config.ResourceGroup,
		vm.ID,
		&armcompute.VirtualMachinesClientGetOptions{Expand: nil})
	if err != nil {
		return nil, err
	}

	foundVM := vmResponse.VirtualMachine

	returnVM := &cloudyvm.VirtualMachineConfiguration{
		ID: *foundVM.ID,
	}

	return returnVM, nil
}

// Find NIC if it already exists
func (vmc *AzureVMController) GetNIC(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) (*cloudyvm.VirtualMachineNetwork, error) {
	nicClient, err := armnetwork.NewInterfacesClient(vmc.Config.SubscriptionID, vmc.cred, &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	})
	if err != nil {
		return nil, err
	}

	opts := &armnetwork.InterfacesClientListAllOptions{}

	pager := nicClient.NewListAllPager(opts)

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, nic := range resp.Value {
			// Match by name
			if nic.Name != nil && strings.Contains(*nic.Name, vm.ID) {
				network := &cloudyvm.VirtualMachineNetwork{
					ID:        *nic.ID,
					Name:      *nic.Name,
					PrivateIP: *nic.Properties.IPConfigurations[0].Properties.PrivateIPAddress,
				}

				return network, nil
			}
		}
	}

	return nil, nil
}

func (vmc *AzureVMController) DeleteNIC(ctx context.Context, vmId string, nicName string) error {
	nicClient, err := armnetwork.NewInterfacesClient(vmc.Config.SubscriptionID, vmc.cred, &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	})
	if err != nil {
		return err
	}

	cloudy.Info(ctx, "[%s] Starting nicClient.BeginDelete '%s' '%s'", vmId, vmc.Config.NetworkResourceGroup, nicName)

	poller, err := nicClient.BeginDelete(ctx, vmc.Config.NetworkResourceGroup, nicName, nil)
	if err != nil {
		_ = cloudy.Error(ctx, "failed to delete the nic: %v", err)
		return err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		_ = cloudy.Error(ctx, "failed to poll while deleting the nic: %v", err)
	}

	return err
}

/*
CreateVirtualMachine

	resource "azurerm_linux_virtual_machine" "main-vm" {
	    name                    = var.vdi-name
	    computer_name           = var.vdi-name
	    admin_username          = "salt"
	    resource_group_name     = var.app-rg-name
	    location                = var.def-location
	    size                    = var.vdi-size
	    source_image_id         = "/subscriptions/${var.subscription-id}/resourceGroups/${var.app-rg-name}/providers/Microsoft.Compute/galleries/${var.source-image-gallery-name}/images/${var.source-image}/versions/${var.source-image-version}"
	    network_interface_ids   = [
	        azurerm_network_interface.main-nic.id,
	    ]

	    admin_ssh_key {
	        username = "salt"
	        public_key = file("${path.module}/vdi-terraform_id_rsa.pub")
	    }

	    os_disk {
	        caching              = "ReadWrite"
	        storage_account_type = "Standard_LRS"
	    }

	    tags = {
	        Application            = "SKYBORG"
	        "Functional Area "     = "VDI"
	        "User Principle Name"  = var.user-principle-name
	    }
	}
*/
func (vmc *AzureVMController) CreateVirtualMachine(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) error {

	// Input Parameters
	region := vmc.Config.Region
	resourceGroup := vmc.Config.ResourceGroup
	vmName := vm.ID

	imageId := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/galleries/%s/images/%s/versions/%s",
		vmc.Config.SubscriptionID,
		vmc.Config.SourceImageGalleryResourceGroup,
		vmc.Config.SourceImageGalleryName,
		vm.Image,
		vm.ImageVersion)

	// What we really need to do here is look through quota and find the best size. But for now we can just use the size specified.
	// TODO: SDK does not include all possible sizes, need to make dynamic
	/* vmSize := findVmSize(vm.Size.Name)
	if vmSize == nil {
		return fmt.Errorf("no matching VM size for %s", vm.Size.Name)
	}*/

	if vm.Size == nil {
		return cloudy.Error(ctx, "[%s] Invalid VM Size %v", vm.ID, vm.Size)
	}
	vmSize := (armcompute.VirtualMachineSizeTypes)(vm.Size.Name)

	diskSizeInGB, err := vmc.ConfigureDiskSize(ctx, vm)
	if err != nil {
		_ = cloudy.Error(ctx, "[%s] Error configuring disk size", vm.ID)
		return err
	}

	// Configure Disk type
	diskType := armcompute.StorageAccountTypesStandardLRS
	if vm.Size.PremiumIO {
		diskType = armcompute.StorageAccountTypesPremiumLRS
	}

	imageReference := parseImageReference(vm, imageId)

	existingVM, err := vmc.GetVM(ctx, vm)
	if err != nil {
		_ = cloudy.Error(ctx, "[%s] Error searching for existing VM", vm.ID)
	}

	tags := map[string]*string{}
	for k, v := range vm.Tags {
		tags[k] = to.Ptr(v)
	}

	vmParameters := armcompute.VirtualMachine{
		Name:     to.Ptr(vmName),
		Location: to.Ptr(region),

		Identity: &armcompute.VirtualMachineIdentity{
			Type: to.Ptr(armcompute.ResourceIdentityTypeNone),
		},

		Tags: tags,
	}

	vmParameters.Properties = &armcompute.VirtualMachineProperties{
		HardwareProfile: &armcompute.HardwareProfile{
			VMSize: &vmSize,
		},
		StorageProfile: &armcompute.StorageProfile{
			ImageReference: imageReference,
		},
	}

	if existingVM != nil {
		cloudy.Info(ctx, "[%s] Existing VM found: %+v", vm.ID, existingVM)
	} else {

		vmOsDiskOsType := vmc.ConfigureVmOsDiskOsTypeType(ctx, vm)
		if vmOsDiskOsType == nil {
			return cloudy.Error(ctx, "[%s] Invalid OS Specified: %s", vm.ID, vm.OSType)
		}

		vmParameters.Properties.StorageProfile.OSDisk = &armcompute.OSDisk{
			OSType:       vmOsDiskOsType,
			DiskSizeGB:   to.Ptr(diskSizeInGB),
			Caching:      to.Ptr(armcompute.CachingTypesReadWrite),
			CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
			ManagedDisk: &armcompute.ManagedDiskParameters{
				StorageAccountType: to.Ptr(diskType),
			},
		}

		vmOsProfile := vmc.ConfigureVmOsProfile(ctx, vm)
		if vmOsProfile == nil {
			return cloudy.Error(ctx, "[%s] Invalid OS Specified: %s", vm.ID, vm.OSType)
		}
		vmParameters.Properties.OSProfile = vmOsProfile

		vmParameters.Properties.NetworkProfile = &armcompute.NetworkProfile{
			NetworkInterfaces: []*armcompute.NetworkInterfaceReference{
				{
					ID: to.Ptr(vm.PrimaryNetwork.ID),
				},
			},
		}
	}

	cloudy.Info(ctx, "[%s] BeginCreateOrUpdate: resourceGroup[%s] vmName[%s] location[%s] vmSize[%s] imageReference[%s] admuser[%s] networkId[%s]",
		vm.ID, resourceGroup, vmName, region, vm.Size.Name, imageId, vm.Credientials.AdminUser, vm.PrimaryNetwork.ID)

	poller, err := vmc.Client.BeginCreateOrUpdate(
		ctx,
		resourceGroup,
		vmName,
		vmParameters,
		nil,
	)
	if err != nil {
		ret, _ := json.Marshal(vmParameters)
		cloudy.Info(ctx, "[%s] VM create error, dumping parameters: %s", vm.ID, string(ret))

		return cloudy.Error(ctx, "[%s] BeginCreateOrUpdate failed to obtain a response: %v", vm.ID, err)
	}
	resp, err := poller.PollUntilDone(context.Background(), &runtime.PollUntilDoneOptions{})
	if err != nil {
		_ = cloudy.Error(ctx, "[%s] PollUntilDone failed to obtain a response: %v", vm.ID, err)
	}

	vm.OSDisk = &cloudyvm.VirtualMachineDisk{
		Name: *resp.VirtualMachine.Properties.StorageProfile.OSDisk.Name,
	}

	cloudy.Info(ctx, "[%s] Created VM ID: %v - %v - %v", vm.ID, *resp.VirtualMachine.ID, resp.VirtualMachine.Properties.ProvisioningState, VMGetPowerState(&resp.VirtualMachine))
	return nil
}

func (vmc *AzureVMController) Delete(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) (*cloudyvm.VirtualMachineConfiguration, error) {
	var err error

	cloudy.Info(ctx, "[%s] Starting Delete (az-vm-create)", vm.ID)

	cloudy.Info(ctx, "[%s] Starting BeginDeallocate", vm.ID)
	deallocatePoller, err := vmc.Client.BeginDeallocate(ctx, vmc.Config.ResourceGroup, vm.ID, nil)

	if err != nil {
		if cloudyazure.Is404(err) {
			cloudy.Info(ctx, "[%s] BeginDeallocate resource not found", vm.ID)
		} else {
			return nil, cloudy.Error(ctx, "[%s] BeginDeallocate unknown error %v", vm.ID, err)
		}

	} else {
		cloudy.Info(ctx, "[%s] Starting deallocatePoller.PollUntilDone", vm.ID)
		_, err = deallocatePoller.PollUntilDone(ctx, nil)
		if err != nil {
			_ = cloudy.Error(ctx, "[%s] Deallocate PollUntilDone failed: %v", vm.ID, err)
			return nil, err
		}

	}

	cloudy.Info(ctx, "[%s] Starting vmc.Client.BeginDelete", vm.ID)
	deletePoller, err := vmc.Client.BeginDelete(ctx, vmc.Config.ResourceGroup, vm.ID, nil)

	if err != nil {
		if cloudyazure.Is404(err) {
			cloudy.Info(ctx, "[%s] BeginDelete resource not found", vm.ID)
		} else {
			return nil, cloudy.Error(ctx, "[%s] BeginDelete unknown error %v", vm.ID, err)
		}
	} else {
		cloudy.Info(ctx, "[%s] Starting deletePoller.PollUntilDone", vm.ID)
		_, err = deletePoller.PollUntilDone(ctx, nil)
		if err != nil {
			_ = cloudy.Error(ctx, "[%s] BeginDelete PollUntilDone failed: %v", vm.ID, err)
			return nil, err
		}
	}

	cloudy.Info(ctx, "[%s] Starting GetVmOsDisk", vm.ID)
	vm.OSDisk, err = vmc.GetVmOsDisk(ctx, vm)
	if err != nil {
		_ = cloudy.Error(ctx, "[%s] failed to find vm os disk: %v", vm.ID, err)
		return nil, err
	}

	if vm.OSDisk != nil {
		cloudy.Info(ctx, "[%s] Starting DeleteVMOSDisk", vm.ID)
		err = vmc.DeleteVMOSDisk(ctx, vm)
		if err != nil {
			_ = cloudy.Error(ctx, "[%s] failed to delete vm os disk: %v", vm.ID, err)
			return nil, err
		}
	} else {
		cloudy.Info(ctx, "[%s] No OS Disk found", vm.ID)
	}

	cloudy.Info(ctx, "[%s] Starting GetNIC", vm.ID)
	vmn, err := vmc.GetNIC(ctx, vm)
	if err != nil {
		_ = cloudy.Error(ctx, "[%s] failed to find vm nic: %v", vm.ID, err)
		return nil, err
	}

	if vmn != nil {
		cloudy.Info(ctx, "[%s] Starting DeleteNIC", vm.ID)
		err = vmc.DeleteNIC(ctx, vm.ID, vmn.Name)
		if err != nil {
			_ = cloudy.Error(ctx, "[%s] failed to delete vm nic: %v", vm.ID, err)
			return nil, err
		}
	} else {
		cloudy.Info(ctx, "[%s] No NIC found", vm.ID)
	}

	return vm, nil
}

func (vmc *AzureVMController) DeleteVM(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) error {
	_, err := vmc.Delete(ctx, vm)
	return err
}

func (vmc *AzureVMController) DeleteVMOSDisk(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) error {
	cloudy.Info(ctx, "[%s] Starting armcompute.NewDisksClient Subscription: %s", vm.ID, vmc.Config.SubscriptionID)
	diskClient, err := armcompute.NewDisksClient(vmc.Config.SubscriptionID, vmc.cred,
		&arm.ClientOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloud.AzureGovernment,
			},
		})
	if err != nil {
		_ = cloudy.Error(ctx, "[%s] failed to create disks client: %v", vm.ID, err)
		return err
	}

	if vmc.Config == nil {
		return cloudy.Error(ctx, "[%s] vmc.config == nil", vm.ID)
	}

	if vm.OSDisk == nil {
		return cloudy.Error(ctx, "[%s] vmc.osdisk == nil", vm.ID)
	}

	cloudy.Info(ctx, "[%s] Starting diskClient.BeginDelete '%s' '%s'", vm.ID, vmc.Config.ResourceGroup, vm.OSDisk.Name)
	pollerResponse, err := diskClient.BeginDelete(ctx, vmc.Config.ResourceGroup, vm.OSDisk.Name, nil)
	if err != nil {
		_ = cloudy.Error(ctx, "[%s] failed to delete vm os %v", vm.ID, err)
		return err
	}

	_, err = pollerResponse.PollUntilDone(ctx, nil)
	if err != nil {
		_ = cloudy.Error(ctx, "[%s] diskClient.BeginDelete PollUntilDone failed to obtain a response: %v", vm.ID, err)
	}

	return err
}

func (vmc *AzureVMController) ConfigureDiskSize(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) (int32, error) {

	// Configure Disk SIze
	sizeInGB := int32(30)
	if vm.OSDisk != nil && vm.OSDisk.Size != "" {
		size, err := strconv.ParseInt(vm.OSDisk.Size, 10, 32)
		if err != nil {
			cloudy.Warn(ctx, "[%s] Invalid Size for OS Disk [%v] using defaul 30GB", vm.ID, vm.OSDisk.Size)
		} else {
			sizeInGB = int32(size)
		}
	}

	// the size of the corresponding disk in the VM image: 127 GB
	// temporarilty setting minimum to 200 for wiggle room
	if strings.EqualFold(vm.OSType, "windows") {
		minWindowsVMSize := int32(200)
		if sizeInGB < minWindowsVMSize {
			sizeInGB = minWindowsVMSize
		}
	}

	return sizeInGB, nil
}

func (vmc *AzureVMController) ConfigureVmOsDiskOsTypeType(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) *armcompute.OperatingSystemTypes {
	if strings.EqualFold(vm.OSType, "windows") {
		return to.Ptr(armcompute.OperatingSystemTypesWindows)
	} else if strings.Contains(strings.ToLower(vm.OSType), "linux") {
		return to.Ptr(armcompute.OperatingSystemTypesLinux)
	}

	return nil
}

func (vmc *AzureVMController) ConfigureVmOsProfile(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) *armcompute.OSProfile {
	if strings.EqualFold(vm.OSType, "windows") {
		return &armcompute.OSProfile{
			ComputerName:         to.Ptr(vm.ID),
			AdminUsername:        to.Ptr(vm.Credientials.AdminUser),
			AdminPassword:        to.Ptr(vm.Credientials.AdminPassword),
			WindowsConfiguration: &armcompute.WindowsConfiguration{},
		}

	} else if strings.Contains(strings.ToLower(vm.OSType), "linux") {
		return &armcompute.OSProfile{
			ComputerName:  to.Ptr(vm.ID),
			AdminUsername: to.Ptr(vm.Credientials.AdminUser),
			LinuxConfiguration: &armcompute.LinuxConfiguration{
				DisablePasswordAuthentication: to.Ptr(true),
				SSH: &armcompute.SSHConfiguration{
					PublicKeys: []*armcompute.SSHPublicKey{
						{
							Path: to.Ptr(fmt.Sprintf("/home/%s/.ssh/authorized_keys",
								vm.Credientials.AdminUser)),
							KeyData: to.Ptr(vm.Credientials.SSHKey),
						},
					},
				},
				ProvisionVMAgent: to.Ptr(true),
			},
			AllowExtensionOperations: to.Ptr(true),
		}
	}

	return nil
}

func (vmc *AzureVMController) GetVMSize(ctx context.Context, size string) (*cloudyvm.VmSize, error) {
	client, err := armcompute.NewResourceSKUsClient(vmc.Config.SubscriptionID, vmc.cred, &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	})
	if err != nil {
		return nil, cloudy.Error(ctx, "could not create NewResourceSKUsClient, %v", err)
	}

	pager := client.NewListPager(&armcompute.ResourceSKUsClientListOptions{})
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, cloudy.Error(ctx, "could not get NextPage, %v", err)
		}

		for _, r := range resp.Value {
			if *r.ResourceType == "virtualMachines" {
				if strings.EqualFold(*r.Name, size) {
					return SizeFromResource(ctx, r), nil
				}
			}
		}
	}

	return nil, fmt.Errorf("size not found: %v", size)
}

// Temporarily (or Permanently) unused
// func findVmSize(size string) *armcompute.VirtualMachineSizeTypes {
// 	for _, s := range armcompute.PossibleVirtualMachineSizeTypesValues() {
// 		if strings.EqualFold(string(s), size) {
// 			return to.Ptr(s)
// 		}
// 	}
// 	return nil
// }

func parseImageReference(vm *cloudyvm.VirtualMachineConfiguration, imageId string) *armcompute.ImageReference {
	imageReference := &armcompute.ImageReference{
		// Version: to.Ptr(vm.ImageVersion), "The property 'imageReference.id' cannot be used together with property 'imageReference.version'."
	}

	if strings.Contains(vm.Image, "::") {
		parts := strings.Split(vm.Image, "::")
		if len(parts) == 3 {
			imageReference.Publisher = to.Ptr(parts[0])
			imageReference.Offer = to.Ptr(parts[1])
			imageReference.SKU = to.Ptr(parts[2])
			imageReference.Version = &vm.ImageVersion
		}
	} else {
		imageReference.ID = to.Ptr(imageId)
	}

	return imageReference
}

func SizeFromResource(ctx context.Context, res *armcompute.ResourceSKU) *cloudyvm.VmSize {
	rtn := &cloudyvm.VmSize{
		Name:   *res.Name,
		Family: *res.Family,
		Size:   *res.Size,
	}

	for _, c := range res.Capabilities {
		if *c.Name == "MaxNetworkInterfaces" {
			rtn.MaxNics, _ = strconv.Atoi(*c.Value)
		}
		if *c.Name == "AcceleratedNetworkingEnabled" {
			rtn.AcceleratedNetworking, _ = strconv.ParseBool(*c.Value)
		}
		if *c.Name == "PremiumIO" {
			rtn.PremiumIO, _ = strconv.ParseBool(*c.Value)
		}
		if *c.Name == "vCPUs" {
			rtn.VCPU, _ = strconv.Atoi(*c.Value)
		}
		if *c.Name == "MemoryGB" {
			rtn.MemoryGB, _ = strconv.ParseFloat(*c.Value, 64)
		}
		if *c.Name == "GPUs" {
			rtn.GPU, _ = strconv.ParseFloat(*c.Value, 64)
		}
	}

	return rtn
}
