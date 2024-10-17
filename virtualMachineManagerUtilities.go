package cloudyazure

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	"github.com/pkg/errors"
)

const (
	vmNameTagKey = "Name"
)

func toResponseError(err error) *azcore.ResponseError {
	var respErr *azcore.ResponseError
	if !errors.As(err, &respErr) {
		return respErr
	}

	return nil
}

func is404(err error) bool {
	respErr := toResponseError(err)

	if respErr != nil && respErr.StatusCode == http.StatusNotFound || bloberror.HasCode(err, bloberror.ResourceNotFound, "ShareNotFound") {
		return true
	}

	return false
}

func pollWrapper[T any](ctx context.Context, poller *runtime.Poller[T], pollerType string) (*T, error) {
	log := logging.GetLogger(ctx)

	ticker := time.NewTicker(5 * time.Second)
	startTime := time.Now()
	defer ticker.Stop()
	defer func() {
		log.InfoContext(ctx, fmt.Sprintf("%s complete (elapsed: %s)", pollerType,
			fmt.Sprintf("%.0f seconds", time.Since(startTime).Seconds())))
	}()

	for {
		select {
		case <-ticker.C:
			log.InfoContext(ctx, fmt.Sprintf("Waiting for %s to complete (elapsed: %s)",
				pollerType, fmt.Sprintf("%.0f seconds", time.Since(startTime).Seconds())))
		default:
			_, err := poller.Poll(ctx)
			if err != nil {
				return nil, errors.Wrapf(err, "pollWrapper: %s (Poll)", pollerType)
			}
			if poller.Done() {
				response, err := poller.Result(ctx)

				if err != nil {
					return nil, errors.Wrapf(err, "pollWrapper: %s (Result)", pollerType)
				}

				return &response, nil
			}
		}
	}
}

func FromCloudyVirtualMachine(ctx context.Context, vm *models.VirtualMachine) armcompute.VirtualMachine {
	log := logging.GetLogger(ctx)

	virtualMachineParameters := armcompute.VirtualMachine{
		// vm Id is saved as ID and Name
		// vm Name is saved in a Tag
		ID:       &vm.ID,
		Name:     &vm.ID,
		Location: &vm.Location.Region,
		Identity: &armcompute.VirtualMachineIdentity{
			Type: to.Ptr(armcompute.ResourceIdentityTypeNone),
		},
	}

	if vm.Tags != nil {
		virtualMachineParameters.Tags = vm.Tags
		virtualMachineParameters.Tags[vmNameTagKey] = &vm.Name
	}

	if vm.Template == nil {
		vm.Template = &models.VirtualMachineTemplate{}
	}

	if vm.Template.Tags != nil {
		for k, v := range vm.Template.Tags {
			_, ok := virtualMachineParameters.Tags[k]

			// Will not overwrite tags already in the VM object
			if !ok {
				virtualMachineParameters.Tags[k] = v
			}
		}
	}

	virtualMachineParameters.Properties = &armcompute.VirtualMachineProperties{

		HardwareProfile: &armcompute.HardwareProfile{
			VMSize: (*armcompute.VirtualMachineSizeTypes)(&vm.Template.Size.ID),
		},
		StorageProfile: &armcompute.StorageProfile{
			ImageReference: &armcompute.ImageReference{
				ID: &vm.OsBaseImageID,
			},
			OSDisk: &armcompute.OSDisk{
				CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
			},
		},
		// SecurityProfile: &armcompute.SecurityProfile{
		// 	SecurityType: to.Ptr(armcompute.SecurityTypesTrustedLaunch),
		// },
	}

	virtualMachineParameters.Properties.OSProfile = &armcompute.OSProfile{
		ComputerName:  to.Ptr(vm.ID),
		AdminUsername: &vm.Template.LocalAdministratorID,
		AdminPassword: to.Ptr(cloudy.GeneratePassword(15, 2, 2, 2)),
	}
	log.InfoContext(ctx, fmt.Sprintf("%+v", virtualMachineParameters.Properties.OSProfile))

	switch vm.Template.OperatingSystem {
	case "windows":
		virtualMachineParameters.Properties.StorageProfile.OSDisk.OSType = to.Ptr(armcompute.OperatingSystemTypesWindows)
		virtualMachineParameters.Properties.OSProfile.WindowsConfiguration = &armcompute.WindowsConfiguration{}
	case "linux":
		virtualMachineParameters.Properties.StorageProfile.OSDisk.OSType = to.Ptr(armcompute.OperatingSystemTypesLinux)
		virtualMachineParameters.Properties.OSProfile.LinuxConfiguration = &armcompute.LinuxConfiguration{
			DisablePasswordAuthentication: to.Ptr(true),
			ProvisionVMAgent:              to.Ptr(true),
		}
		virtualMachineParameters.Properties.OSProfile.AllowExtensionOperations = to.Ptr(true)

	}

	nics := []*armcompute.NetworkInterfaceReference{}

	for _, cloudyNic := range vm.Nics {
		nic := &armcompute.NetworkInterfaceReference{
			ID: &cloudyNic.ID,
		}

		nics = append(nics, nic)
	}

	virtualMachineParameters.Properties.NetworkProfile = &armcompute.NetworkProfile{
		NetworkInterfaces: nics,
	}

	return virtualMachineParameters
}

func ToCloudyVirtualMachine(vm *armcompute.VirtualMachine) *models.VirtualMachine {

	cloudyVm := models.VirtualMachine{
		ID:   *vm.ID,
		Name: *vm.Name,
		Location: &models.VirtualMachineLocation{
			Region: *vm.Location,
		},
		Template: &models.VirtualMachineTemplate{},
		Tags:     map[string]*string{},
	}

	if vm.Properties != nil {
		cloudyVm.State = *vm.Properties.ProvisioningState

		if vm.Properties.InstanceView != nil {
			for _, status := range vm.Properties.InstanceView.Statuses {
				if strings.Contains(*status.Code, "PowerState") {
					statusParts := strings.Split(*status.Code, "/")
					cloudyVm.Status = statusParts[1]
				}
			}
		}

		if vm.Properties.HardwareProfile != nil {
			cloudyVm.Template.Size = &models.VirtualMachineSize{
				Name: string(*vm.Properties.HardwareProfile.VMSize),
			}
		}

		if vm.Properties.NetworkProfile != nil {
			nics := []*models.VirtualMachineNic{}
			for _, nic := range vm.Properties.NetworkProfile.NetworkInterfaces {
				nics = append(nics, &models.VirtualMachineNic{ID: *nic.ID})
			}
			cloudyVm.Nics = nics
		}

		if vm.Properties.StorageProfile != nil {
			if vm.Properties.StorageProfile.OSDisk != nil {
				cloudyVm.Template.OperatingSystem = string(*vm.Properties.StorageProfile.OSDisk.OSType)

				cloudyVm.OsDisk = &models.VirtualMachineDisk{
					ID:     *vm.Properties.StorageProfile.OSDisk.ManagedDisk.ID,
					OsDisk: true,
				}

				if vm.Properties.StorageProfile.OSDisk.DiskSizeGB != nil {
					cloudyVm.OsDisk.Size = int64(*vm.Properties.StorageProfile.OSDisk.DiskSizeGB)
				}
			}

			if vm.Properties.StorageProfile.DataDisks != nil {
				disks := []*models.VirtualMachineDisk{}
				for _, disk := range vm.Properties.StorageProfile.DataDisks {
					disks = append(disks, &models.VirtualMachineDisk{
						ID:     *disk.ManagedDisk.ID,
						OsDisk: false,
						Size:   int64(*disk.DiskSizeGB),
					})
				}
				cloudyVm.Disks = disks
			}
		}
	}

	if vm.Tags != nil {
		for k, v := range vm.Tags {
			if strings.EqualFold(k, vmNameTagKey) {
				cloudyVm.Name = *v
			} else {
				cloudyVm.Tags[k] = v
			}
		}
	}

	return &cloudyVm
}

func ToCloudyVirtualMachineSize(ctx context.Context, resource *armcompute.ResourceSKU) *models.VirtualMachineSize {

	log := logging.GetLogger(ctx)

	size := models.VirtualMachineSize{
		ID:   *resource.Name,
		Name: *resource.Name,
		Family: &models.VirtualMachineFamily{
			ID:   *resource.Family,
			Name: *resource.Family,
		},
	}

	locations := map[string]*models.VirtualMachineLocation{}

	for _, location := range resource.Locations {
		_, ok := locations[*location]
		if !ok {
			locations[*location] = ToCloudyVirtualMachineLocation(location)
		}
	}
	size.Locations = locations

	for _, capability := range resource.Capabilities {
		switch *capability.Name {
		case "vCPUs":
			v, err := strconv.ParseInt(*capability.Value, 10, 64)
			if err != nil {
				log.ErrorContext(ctx, fmt.Sprintf("capability error: %s %s", *capability.Name, *capability.Value), logging.WithError(err))
				continue
			}

			size.CPU = v

		case "GPUs":
			v, err := strconv.ParseInt(*capability.Value, 10, 64)
			if err != nil {
				log.ErrorContext(ctx, fmt.Sprintf("capability error: %s %s", *capability.Name, *capability.Value), logging.WithError(err))
				continue
			}

			size.Gpu = v

		case "MemoryGB":
			v, err := strconv.ParseFloat(*capability.Value, 64)
			if err != nil {
				log.ErrorContext(ctx, fmt.Sprintf("capability error: %s %s", *capability.Name, *capability.Value), logging.WithError(err))
				continue
			}
			size.RAM = v

		case "MaxDataDiskCount":
			v, err := strconv.ParseInt(*capability.Value, 10, 64)
			if err != nil {
				log.ErrorContext(ctx, fmt.Sprintf("capability error: %s %s", *capability.Name, *capability.Value), logging.WithError(err))
				continue
			}

			size.MaxDataDisks = v

		case "MaxNetworkInterfaces":
			v, err := strconv.ParseInt(*capability.Value, 10, 64)
			if err != nil {
				log.ErrorContext(ctx, fmt.Sprintf("capability error: %s %s", *capability.Name, *capability.Value), logging.WithError(err))
				continue
			}

			size.MaxNetworkInterfaces = v

		case "AcceleratedNetworkingEnabled":
			v, err := strconv.ParseBool(*capability.Value)
			if err != nil {
				log.ErrorContext(ctx, fmt.Sprintf("capability error: %s %s", *capability.Name, *capability.Value), logging.WithError(err))
				continue
			}

			size.AcceleratedNetworking = v

		case "PremiumIO":
			v, err := strconv.ParseBool(*capability.Value)
			if err != nil {
				log.ErrorContext(ctx, fmt.Sprintf("capability error: %s %s", *capability.Name, *capability.Value), logging.WithError(err))
				continue
			}

			size.PremiumIo = v

		case "MaxResourceVolumeMB",
			"OSVhdSizeMB",
			"MemoryPreservingMaintenanceSupported",
			"HyperVGenerations",
			"CpuArchitectureType",
			"LowPriorityCapable",
			"VMDeploymentTypes",
			"vCPUsAvailable",
			"ACUs",
			"vCPUsPerCore",
			"CombinedTempDiskAndCachedIOPS",
			"CombinedTempDiskAndCachedReadBytesPerSecond",
			"CombinedTempDiskAndCachedWriteBytesPerSecond",
			"UncachedDiskIOPS",
			"UncachedDiskBytesPerSecond",
			"EphemeralOSDiskSupported",
			"SupportedEphemeralOSDiskPlacements",
			"EncryptionAtHostSupported",
			"CapacityReservationSupported",
			"CachedDiskBytes",
			"UltraSSDAvailable",
			"MaxWriteAcceleratorDisksAllowed",
			"TrustedLaunchDisabled",
			"ParentSize",
			"DiskControllerTypes",
			"NvmeDiskSizeInMiB",
			"NvmeSizePerDiskInMiB",
			"HibernationSupported",
			"RdmaEnabled":

			// These capabilities may be used later
			continue

		default:
			log.InfoContext(ctx, fmt.Sprintf("unhandled capability: %s %s", *capability.Name, *capability.Value))

		}

	}

	return &size
}

func ToCloudyVirtualMachineLocation(location *string) *models.VirtualMachineLocation {
	return &models.VirtualMachineLocation{
		Cloud:  "azure",
		Region: *location,
	}
}
