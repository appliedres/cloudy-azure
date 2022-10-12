package cloudyazure

import (
	"context"
	"encoding/base64"
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
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"

	"github.com/appliedres/cloudy"
	cloudyvm "github.com/appliedres/cloudy/vm"
)

func (vmc *AzureVMController) Create(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) (*cloudyvm.VirtualMachineConfiguration, error) {
	err := vmc.ValidateConfiguration(ctx, vm)
	if err != nil {
		return vm, err
	}

	// Check if NIC already exists
	cloudy.Info(ctx, "[%s] Starting GetNIC", vm.ID)
	network, err := vmc.GetNIC(ctx, vm)
	if err != nil {
		cloudy.Error(ctx, "[%s] Error looking for NIC: %s", vm.ID, err.Error())
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
			return vm, fmt.Errorf("no available subnets")
		}

		// Check / Create the Network Interface
		cloudy.Info(ctx, "[%s] Starting CreateNIC", vm.ID)
		err = vmc.CreateNIC(ctx, vm, subnetId)
		if err != nil {
			return vm, err
		}
	}

	if strings.Contains(strings.ToLower(vm.OSType), "linux") {
		cloudy.Info(ctx, "[%s] Starting CreateLinuxVirtualMachine", vm.ID)
		err = vmc.CreateLinuxVirtualMachine(ctx, vm)
		if err != nil {
			cloudy.Error(ctx, "[%s] CreateLinuxVirtualMachine err: %s", vm.ID, err.Error())
		}
		return vm, err
	} else if strings.EqualFold(vm.OSType, "windows") {
		cloudy.Info(ctx, "[%s] Starting CreateWindowsVirtualMachine", vm.ID)
		// Temp Overwrite of Admin Password to random string
		vm.Credientials.AdminPassword = cloudy.GeneratePassword(16, 1, 1, 1)
		err = vmc.CreateWindowsVirtualMachine(ctx, vm)
		if err != nil {
			cloudy.Error(ctx, "[%s] CreateWindowsVirtualMachine err: %s", vm.ID, err.Error())
		}
		return vm, err
	}

	return VmCreate(ctx, vmc.Client, vm)
}

func (vmc *AzureVMController) ValidateConfiguration(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) error {
	if strings.Contains(strings.ToLower(vm.OSType), "linux") {
	} else if strings.EqualFold(vm.OSType, "windows") {
	} else {
		return cloudy.Error(ctx, "invalid OS Type: %v, cannot create vm", vm.OSType)
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

	nsg, err := vmc.GetNSG(ctx, vmc.Config.NetworkSecurityGroupName)
	if err != nil {
		return err
	}

	if vm.Size == nil {
		return cloudy.Error(ctx, "Invalid VM Size %v", vm.Size)
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
								NetworkSecurityGroup: &armnetwork.SecurityGroup{
									ID: nsg.ID,
								},
							},
						},
						PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
					},
				},
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

func (vmc *AzureVMController) DeleteNIC(ctx context.Context, nicName string) error {
	nicClient, err := armnetwork.NewInterfacesClient(vmc.Config.SubscriptionID, vmc.cred, &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	})
	if err != nil {
		return err
	}
	_, err = nicClient.BeginDelete(ctx, vmc.Config.ResourceGroup, nicName, nil)
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
func (vmc *AzureVMController) CreateLinuxVirtualMachine(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) error {

	// Input Parameters
	region := vmc.Config.Region
	subscriptionId := vmc.Config.SubscriptionID
	resourceGroup := vmc.Config.ResourceGroup
	imageGalleryName := vmc.Config.SourceImageGalleryName
	imageName := vm.Image
	imageVersion := vm.ImageVersion
	vmName := vm.ID

	tags := map[string]*string{}
	for k, v := range vm.Tags {
		tags[k] = to.Ptr(v)
	}

	// What we really need to do here is look through quota and find the best size. But for now we can just use the size specified.
	// TODO: SDK does not include all possible sizes, need to make dynamic
	/* vmSize := findVmSize(vm.Size.Name)
	if vmSize == nil {
		return fmt.Errorf("no matching VM size for %s", vm.Size.Name)
	}*/

	vmSize := (armcompute.VirtualMachineSizeTypes)(vm.Size.Name)

	imageId := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/galleries/%s/images/%s/versions/%s", subscriptionId, resourceGroup, imageGalleryName, imageName, imageVersion)
	adminUser := vm.Credientials.AdminUser
	//adminPassword := vm.Credientials.AdminPassword
	sshKey := vm.Credientials.SSHKey

	if vm.Size == nil {
		return cloudy.Error(ctx, "[%s] Invalid VM Size %v", vm.ID, vm.Size)
	}

	// Configure Disk SIze
	sizeinGB := int32(30)
	if vm.OSDisk != nil && vm.OSDisk.Size != "" {
		size, err := strconv.ParseInt(vm.OSDisk.Size, 10, 32)
		if err != nil {
			cloudy.Warn(ctx, "[%s] Invalid Size for OS Disk [%v] using defaul 30GB", vm.ID, vm.OSDisk.Size)
		} else {
			sizeinGB = int32(size)
		}
	}

	// Configure Disk type
	diskType := armcompute.StorageAccountTypesStandardLRS
	if vm.Size.PremiumIO {
		diskType = armcompute.StorageAccountTypesPremiumLRS
	}

	imageReference := parseImageReference(vm, imageId)

	client := vmc.Client
	cloudy.Info(ctx, "[%s] BeginCreateOrUpdate: resourceGroup[%s] vmName[%s] location[%s] vmSize[%s] imageReference[%s] admuser[%s] networkId[%s]", vm.ID, resourceGroup, vmName, region, vm.Size.Name, imageId, adminUser, vm.PrimaryNetwork.ID)

	poller, err := client.BeginCreateOrUpdate(
		ctx,
		resourceGroup,
		vmName,
		armcompute.VirtualMachine{
			Name:     to.Ptr(vmName),
			Location: to.Ptr(region),

			Identity: &armcompute.VirtualMachineIdentity{
				Type: to.Ptr(armcompute.ResourceIdentityTypeNone),
			},

			Properties: &armcompute.VirtualMachineProperties{

				HardwareProfile: &armcompute.HardwareProfile{
					VMSize: &vmSize,
				},

				StorageProfile: &armcompute.StorageProfile{
					ImageReference: imageReference,
					OSDisk: &armcompute.OSDisk{
						OSType:       to.Ptr(armcompute.OperatingSystemTypesLinux),
						DiskSizeGB:   to.Ptr(sizeinGB),
						Caching:      to.Ptr(armcompute.CachingTypesReadWrite),
						CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
						ManagedDisk: &armcompute.ManagedDiskParameters{
							StorageAccountType: to.Ptr(diskType),
						},
					},
				},

				OSProfile: &armcompute.OSProfile{
					ComputerName:  to.Ptr(vmName),
					AdminUsername: to.Ptr(adminUser),
					//AdminPassword: to.Ptr(adminPassword),
					LinuxConfiguration: &armcompute.LinuxConfiguration{
						DisablePasswordAuthentication: to.Ptr(true),
						SSH: &armcompute.SSHConfiguration{
							PublicKeys: []*armcompute.SSHPublicKey{
								{
									Path:    to.Ptr(fmt.Sprintf("/home/%s/.ssh/authorized_keys", adminUser)),
									KeyData: to.Ptr(sshKey),
								},
							},
						},
						ProvisionVMAgent: to.Ptr(true),
					},
					AllowExtensionOperations: to.Ptr(true),
				},

				NetworkProfile: &armcompute.NetworkProfile{
					NetworkInterfaces: []*armcompute.NetworkInterfaceReference{
						{
							ID: to.Ptr(vm.PrimaryNetwork.ID),
						},
					},
				},
			},
			Tags: tags,
		},
		nil,
	)
	if err != nil {
		// var azErr *azcore.ResponseError
		// if errors.As(err, azErr) {
		// 	azErr.RawResponse.Body
		// }

		return cloudy.Error(ctx, "[%s] failed to obtain a response: %v", vm.ID, err)
	}
	resp, err := poller.PollUntilDone(context.Background(), &runtime.PollUntilDoneOptions{})
	if err != nil {
		cloudy.Error(ctx, "[%s] failed to obtain a response: %v", vm.ID, err)
	}

	vm.OSDisk = &cloudyvm.VirtualMachineDisk{
		Name: *resp.VirtualMachine.Properties.StorageProfile.OSDisk.Name,
	}

	cloudy.Info(ctx, "Created VM ID: %v - %v - %v", *resp.VirtualMachine.ID, resp.VirtualMachine.Properties.ProvisioningState, VMGetPowerState(&resp.VirtualMachine))
	return nil
}

func (vmc *AzureVMController) DeleteVM(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) error {
	// Try to terminate the VM if it is running
	// resp, err := vmc.Client.Get(ctx, vmc.Config.ResourceGroup, vmName, nil)
	// if err != nil {
	// 	cloudy.Error(ctx, "failed to obtain a response: %v", err)
	// 	return err
	// }

	vmName := vm.Name

	deallocatePoller, err := vmc.Client.BeginDeallocate(ctx, vmc.Config.ResourceGroup, vmName, nil)
	if err != nil {
		cloudy.Error(ctx, "failed to obtain a response: %v", err)
		return err
	}
	_, err = deallocatePoller.PollUntilDone(ctx, nil)
	if err != nil {
		cloudy.Error(ctx, "failed to deallocate: %v", err)
		return err
	}

	deletePoller, err := vmc.Client.BeginDelete(ctx, vmc.Config.ResourceGroup, vmName, nil)
	if err != nil {
		cloudy.Error(ctx, "failed to obtain a response: %v", err)
	}

	_, err = deletePoller.PollUntilDone(ctx, nil)
	if err != nil {
		cloudy.Error(ctx, "failed to delete: %v", err)
		return err
	}

	vmc.DeleteVMOSDisk(ctx, vm)

	return err
}

func (vmc *AzureVMController) DeleteVMOSDisk(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) error {
	diskClient, err := armcompute.NewDisksClient(vmc.Config.SubscriptionID, vmc.cred,
		&arm.ClientOptions{
			ClientOptions: policy.ClientOptions{
				Cloud: cloud.AzureGovernment,
			},
		})
	if err != nil {
		cloudy.Error(ctx, "failed to create disks client: %v", err)
	}

	pollerResponse, err := diskClient.BeginDelete(ctx, vmc.Config.ResourceGroup, vm.OSDisk.Name, nil)
	if err != nil {
		cloudy.Error(ctx, "failed to terminate: %v", err)
	}

	_, err = pollerResponse.PollUntilDone(ctx, nil)
	if err != nil {
		cloudy.Error(ctx, "failed to obtain a response: %v", err)
	}

	return err
}

/*
resource "azurerm_windows_virtual_machine" "main-vm" {
    name                    = var.vdi-name
    computer_name           = var.vdi-name
    resource_group_name     = var.app-rg-name
    location                = var.def-location
    size                    = var.vdi-size
    source_image_id         = "/subscriptions/${var.subscription-id}/resourceGroups/${var.app-rg-name}/providers/Microsoft.Compute/galleries/${var.source-image-gallery-name}/images/${var.source-image}/versions/${var.source-image-version}"
    network_interface_ids   = [
        azurerm_network_interface.main-nic.id,
    ]

    admin_username          = var.vm-admin-username
    admin_password          = random_password.admin_password.result

    os_disk {
        caching              = "ReadWrite"
        storage_account_type = "StandardSSD_LRS"
    }

    tags = {
        Application            = "SKYBORG"
        "Functional Area "     = "VDI"
        "User Principle Name"  = var.user-principle-name
    }

}*/
func (vmc *AzureVMController) CreateWindowsVirtualMachine(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) error {

	// Input Parameters
	region := vmc.Config.Region
	subscriptionId := vmc.Config.SubscriptionID
	resourceGroup := vmc.Config.ResourceGroup
	imageGalleryName := vmc.Config.SourceImageGalleryName
	imageName := vm.Image
	imageVersion := vm.ImageVersion
	vmName := vm.ID

	tags := map[string]*string{}
	for k, v := range vm.Tags {
		tags[k] = to.Ptr(v)
	}

	// What we really need to do here is look through quota and find the best size. But for now we can just use the size specified.
	// TODO: SDK does not include all possible sizes, need to make dynamic
	// vmSize := findVmSize(vm.Size.Name)
	// if vmSize == nil {
	// 	return cloudy.Error(ctx, "[%s] no matching VM size for %s", vm.ID, vm.Size.Name)
	// }

	vmSize := (armcompute.VirtualMachineSizeTypes)(vm.Size.Name)

	imageId := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/galleries/%s/images/%s/versions/%s", subscriptionId, resourceGroup, imageGalleryName, imageName, imageVersion)
	adminUser := vm.Credientials.AdminUser
	adminPassword := vm.Credientials.AdminPassword

	// Configure Disk SIze
	sizeinGB := int32(30)
	if vm.OSDisk != nil && vm.OSDisk.Size != "" {
		size, err := strconv.ParseInt(vm.OSDisk.Size, 10, 32)
		if err != nil {
			cloudy.Warn(ctx, "[%s] Invalid Size for OS Disk [%v] using defaul 30GB", vm.ID, vm.OSDisk.Size)
		} else {
			sizeinGB = int32(size)
		}
	}

	// Configure Disk type
	diskType := armcompute.StorageAccountTypesStandardLRS
	if vm.Size.PremiumIO {
		diskType = armcompute.StorageAccountTypesPremiumLRS
	}

	imageReference := parseImageReference(vm, imageId)

	client := vmc.Client
	cloudy.Info(ctx, "[%s] BeginCreateOrUpdate: resourceGroup[%s] vmName[%s] location[%s] vmSize[%s] imageReference[%s] admuser[%s] networkId[%s]", vm.ID, resourceGroup, vmName, region, vm.Size.Name, imageId, adminUser, vm.PrimaryNetwork.ID)

	poller, err := client.BeginCreateOrUpdate(
		ctx,
		resourceGroup,
		vmName,
		armcompute.VirtualMachine{
			Name:     to.Ptr(vmName),
			Location: to.Ptr(region),

			Properties: &armcompute.VirtualMachineProperties{

				HardwareProfile: &armcompute.HardwareProfile{
					VMSize: &vmSize,
				},

				StorageProfile: &armcompute.StorageProfile{
					ImageReference: imageReference,
					OSDisk: &armcompute.OSDisk{
						OSType:       to.Ptr(armcompute.OperatingSystemTypesWindows),
						DiskSizeGB:   to.Ptr(sizeinGB),
						Caching:      to.Ptr(armcompute.CachingTypesReadWrite),
						CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
						ManagedDisk: &armcompute.ManagedDiskParameters{
							StorageAccountType: to.Ptr(diskType),
						},
					},
				},

				OSProfile: &armcompute.OSProfile{
					ComputerName:         to.Ptr(vmName),
					AdminUsername:        to.Ptr(adminUser),
					AdminPassword:        to.Ptr(adminPassword),
					WindowsConfiguration: &armcompute.WindowsConfiguration{},
				},

				NetworkProfile: &armcompute.NetworkProfile{
					NetworkInterfaces: []*armcompute.NetworkInterfaceReference{
						{
							ID: to.Ptr(vm.PrimaryNetwork.ID),
							Properties: &armcompute.NetworkInterfaceReferenceProperties{
								Primary: to.Ptr(true),
							},
						},
					},
				},
			},
			Tags: tags,
		},
		nil,
	)
	if err != nil {
		return cloudy.Error(ctx, "[%s] failed to obtain a response: %v", vm.ID, err)
	}

	resp, err := poller.PollUntilDone(context.Background(), &runtime.PollUntilDoneOptions{})
	if err != nil {
		return cloudy.Error(ctx, "[%s] failed to obtain a response: %v", vm.ID, err)
	}

	vm.OSDisk = &cloudyvm.VirtualMachineDisk{
		Name: *resp.VirtualMachine.Properties.StorageProfile.OSDisk.Name,
	}

	cloudy.Info(ctx, "Created VM ID: %v - %v - %v", *resp.VirtualMachine.ID, resp.VirtualMachine.Properties.ProvisioningState, VMGetPowerState(&resp.VirtualMachine))
	return nil
}

func (vmc *AzureVMController) AddADJoinExtensionWindows(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) error {
	AdDomainName, err := vmc.Vault.GetSecret(ctx, "AdDomainName")
	if err != nil {
		return cloudy.Error(ctx, "could not  read AdDomainName from vault, %v", err)
	}
	AdJoinUser, err := vmc.Vault.GetSecret(ctx, "AdJoinUser")
	if err != nil {
		return cloudy.Error(ctx, "could not  read AdDomainName from vault, %v", err)
	}
	AdJoinPassword, err := vmc.Vault.GetSecret(ctx, "AdJoinPassword")
	if err != nil {
		return cloudy.Error(ctx, "could not read AdJoinUser from vault, %v", err)
	}

	client, err := armcompute.NewVirtualMachineExtensionsClient(vmc.Config.SubscriptionID, vmc.cred, &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	})
	if err != nil {
		return cloudy.Error(ctx, "could not create extensions client")
	}

	poller, err := client.BeginCreateOrUpdate(ctx, vmc.Config.ResourceGroup, vm.ID, "ADJoin", armcompute.VirtualMachineExtension{
		Location: &vmc.Config.Region,
		Properties: &armcompute.VirtualMachineExtensionProperties{
			Publisher:               to.Ptr("Microsoft.Compute"),
			AutoUpgradeMinorVersion: to.Ptr(true),
			Type:                    to.Ptr("JsonADDomainExtension"),
			TypeHandlerVersion:      to.Ptr("1.3"),
			Settings: &map[string]interface{}{
				"Name":    AdDomainName,
				"User":    AdJoinUser,
				"Restart": "true",
				"Options": "3",
			},
			ProtectedSettings: &map[string]interface{}{
				"Password": AdJoinPassword,
			},
		},
	}, nil)
	if err != nil {
		return cloudy.Error(ctx, "could not create adjoin extension: %v", err)
	}
	resp, err := poller.PollUntilDone(context.Background(), &runtime.PollUntilDoneOptions{})
	if err != nil {
		cloudy.Error(ctx, "failed to obtain a response: %v", err)
	}
	cloudy.Info(ctx, "Created ADJoin Extension: %v", *resp.ID)
	return nil
}

func (vmc *AzureVMController) AddInstallSaltMinionExt(ctx context.Context, vm *cloudyvm.VirtualMachineConfiguration) error {
	// windowsSaltCommandb64 := "${base64encode(data.template_file.tfSalt.rendered)}"
	windowsSaltCommandb64 := base64.StdEncoding.EncodeToString([]byte(WindowsSaltInstallCmd))
	saltCmd := vmc.Config.SaltCmd
	fullCmd := fmt.Sprintf("powershell -command \"[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('%v')) | Out-File -filepath install.ps1\" && powershell -ExecutionPolicy Unrestricted -File install.ps1 %s", windowsSaltCommandb64, saltCmd)

	// Look up from keyvault
	AdDomainName, err := vmc.Vault.GetSecret(ctx, "AdDomainName")
	if err != nil {
		return cloudy.Error(ctx, "could not  read AdDomainName from vault, %v", err)
	}
	AdJoinUser, err := vmc.Vault.GetSecret(ctx, "AdJoinUser")
	if err != nil {
		return cloudy.Error(ctx, "could not  read AdDomainName from vault, %v", err)
	}

	client, err := armcompute.NewVirtualMachineExtensionsClient(vmc.Config.SubscriptionID, vmc.cred, &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	})
	if err != nil {
		return cloudy.Error(ctx, "could not create extensions client")
	}

	poller, err := client.BeginCreateOrUpdate(ctx, vmc.Config.ResourceGroup, vm.ID, "SaltMinion", armcompute.VirtualMachineExtension{
		Location: &vmc.Config.Region,
		Properties: &armcompute.VirtualMachineExtensionProperties{
			Publisher:               to.Ptr("Microsoft.Compute"),
			AutoUpgradeMinorVersion: to.Ptr(true),
			Type:                    to.Ptr("CustomScriptExtension"),
			TypeHandlerVersion:      to.Ptr("1.9"),
			Settings: &map[string]interface{}{
				"Name":    AdDomainName,
				"User":    AdJoinUser,
				"Restart": "true",
				"Options": "3",
			},
			ProtectedSettings: &map[string]interface{}{
				"commandToExecute": fullCmd,
			},
		},
	}, nil)
	if err != nil {
		return cloudy.Error(ctx, "could not create adjoin extension: %v", err)
	}
	resp, err := poller.PollUntilDone(context.Background(), &runtime.PollUntilDoneOptions{})
	if err != nil {
		cloudy.Error(ctx, "failed to obtain a response: %v", err)
	}
	cloudy.Info(ctx, "Created ADJoin Extension: %v", *resp.ID)
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

func findVmSize(size string) *armcompute.VirtualMachineSizeTypes {
	for _, s := range armcompute.PossibleVirtualMachineSizeTypesValues() {
		if strings.EqualFold(string(s), size) {
			return to.Ptr(s)
		}
	}
	return nil
}

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

var WindowsSaltInstallCmd = `
[CmdletBinding()]
Param(
    [Parameter(Mandatory = $false, ValueFromPipeline = $true)]
    [string]$minion = "not-specified",

    [Parameter(Mandatory = $false, ValueFromPipeline = $true)]
    [string]$master = "not-specified",

    [Parameter(Mandatory = $false, ValueFromPipeline = $true)]
    [string]$masterkey = "not-specified",

    [Parameter(Mandatory = $false, ValueFromPipeline = $true)]
    [string]$saltUrl = "not-specified",

    [Parameter(Mandatory = $false, ValueFromPipeline = $true)]
    [string]$defaultminionurl = "not-specified"
)
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.SecurityProtocolType]'Tls12'
$webclient = New-Object System.Net.WebClient
New-Item C:\tmp\ -ItemType directory -Force | Out-Null
# Download default minion 
If ($defaultminionurl -ne "not-specified") {
    $webclient.DownloadFile($defaultminionurl, 'c:\tmp\minion')
}
If (Test-Path C:\tmp\minion.pem) {
    New-Item C:\salt\conf\pki\minion\ -ItemType Directory -Force | Out-Null
    Copy-Item -Path C:\tmp\minion.pem -Destination C:\salt\conf\pki\minion\ -Force | Out-Null
    Copy-Item -Path C:\tmp\minion.pub -Destination C:\salt\conf\pki\minion\ -Force | Out-Null
    }

If (Test-Path C:\tmp\minion) {
    New-Item C:\salt\conf\ -ItemType Directory -Force | Out-Null
    Copy-Item -Path C:\tmp\minion -Destination C:\salt\conf\ -Force | Out-Null
}
If (Test-Path C:\tmp\grains) {
    New-Item C:\salt\conf\ -ItemType Directory -Force | Out-Null
    Copy-Item -Path C:\tmp\grains -Destination C:\salt\conf\ -Force | Out-Null
}
#dl/install
$saltExe = "Salt-Minion-Setup.exe"
$file = "C:\tmp\$saltExe"
If ($saltUrl -ne "not-specified") {$webclient.DownloadFile($saltUrl, $file)}
$parameters = ""
If ($minion -ne "not-specified") { $parameters = "/minion-name=$minion" }
If ($master -ne "not-specified") { $parameters = "$parameters /master=$master" }
Write-Output "Salt Installing"
Start-Process $file -ArgumentList "/S $parameters" -Wait -NoNewWindow -PassThru 
Write-Output "Salt Installed"
#install service
$service = Get-Service salt-minion -ErrorAction SilentlyContinue
While (!$service) {
    Start-Sleep -s 2
    $service = Get-Service salt-minion -ErrorAction SilentlyContinue
}
Start-Service -Name "salt-minion" -ErrorAction SilentlyContinue
$try = 0
While (($service.Status -ne "Running") -and ($try -ne 4)) {
    Start-Service -Name "salt-minion" -ErrorAction SilentlyContinue
    $service = Get-Service salt-minion -ErrorAction SilentlyContinue
    Start-Sleep -s 2
    $try += 1
}
If ($service.Status -eq "Stopped") {
    Write-Output -NoNewline "Failed to start salt minion"
    exit 1
}
Write-Output "Salt Complete"
`
