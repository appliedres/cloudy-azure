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

	// Cloudy VM States
	CREATING = "creating"
	STARTING = "starting"
	RUNNING = "running"
	UPDATING = "updating"
	STOPPING = "stopping"
	STOPPED = "stopped"
	DELETING = "deleting"
	DELETED = "deleted"
	RESTARTING = "restarting"
	FAILED = "failed"
	UNKNOWN = "unknown"
)

func toResponseError(err error) *azcore.ResponseError {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
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

func FromCloudyVirtualMachine(ctx context.Context, cloudyVM *models.VirtualMachine) armcompute.VirtualMachine {
	log := logging.GetLogger(ctx)

	azVM := armcompute.VirtualMachine{
		// cloudyVM Id is saved as ID and Name
		// cloudyVM Name is saved in a Tag
		ID:       &cloudyVM.ID,
		Name:     &cloudyVM.ID,
		Location: &cloudyVM.Location.Region,
		Identity: &armcompute.VirtualMachineIdentity{
			Type: to.Ptr(armcompute.ResourceIdentityTypeNone),
		},
	}

	if cloudyVM.Tags != nil {
		azVM.Tags = cloudyVM.Tags
	} else {
		azVM.Tags = make(map[string]*string)
	}
	azVM.Tags[vmNameTagKey] = &cloudyVM.Name

	if cloudyVM.Template == nil {
		cloudyVM.Template = &models.VirtualMachineTemplate{}
	}

	if cloudyVM.Template.Tags != nil {
		for k, v := range cloudyVM.Template.Tags {
			_, ok := azVM.Tags[k]

			// Will not overwrite tags already in the VM object
			if !ok {
				azVM.Tags[k] = v
			}
		}
	}

	azVM.Properties = &armcompute.VirtualMachineProperties{

		HardwareProfile: &armcompute.HardwareProfile{
			VMSize: (*armcompute.VirtualMachineSizeTypes)(&cloudyVM.Template.Size.ID),
		},
		StorageProfile: &armcompute.StorageProfile{
			ImageReference: &armcompute.ImageReference{
				ID: &cloudyVM.OsBaseImageID,
			},
			OSDisk: &armcompute.OSDisk{
				CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
			},
		},
		SecurityProfile: &armcompute.SecurityProfile{
			SecurityType: to.Ptr(armcompute.SecurityTypesTrustedLaunch),
		},
	}

	azVM.Properties.OSProfile = &armcompute.OSProfile{
		ComputerName:  to.Ptr(cloudyVM.ID),
		AdminUsername: &cloudyVM.Template.LocalAdministratorID,
		AdminPassword: to.Ptr(cloudy.GeneratePassword(15, 2, 2, 2)),
	}
	log.InfoContext(ctx, fmt.Sprintf("%+v", azVM.Properties.OSProfile))

	switch cloudyVM.Template.OperatingSystem {
	case "windows":
		azVM.Properties.StorageProfile.OSDisk.OSType = to.Ptr(armcompute.OperatingSystemTypesWindows)
		azVM.Properties.OSProfile.WindowsConfiguration = &armcompute.WindowsConfiguration{}
	case "linux":
		azVM.Properties.StorageProfile.OSDisk.OSType = to.Ptr(armcompute.OperatingSystemTypesLinux)
		azVM.Properties.OSProfile.LinuxConfiguration = &armcompute.LinuxConfiguration{
			DisablePasswordAuthentication: to.Ptr(true),
			ProvisionVMAgent:              to.Ptr(true),
		}
		azVM.Properties.OSProfile.AllowExtensionOperations = to.Ptr(true)

	}

	nics := []*armcompute.NetworkInterfaceReference{}

	for _, cloudyNic := range cloudyVM.Nics {
		nic := &armcompute.NetworkInterfaceReference{
			ID: &cloudyNic.ID,
		}

		nics = append(nics, nic)
	}

	azVM.Properties.NetworkProfile = &armcompute.NetworkProfile{
		NetworkInterfaces: nics,
	}

	return azVM
}

func ToCloudyVirtualMachine(ctx context.Context, azVM *armcompute.VirtualMachine) *models.VirtualMachine {
	log := logging.GetLogger(ctx)

	cloudyVm := models.VirtualMachine{
		ID:   *azVM.Name,  // Azure VM Name is cloudy ID, in UVM-<alphanumeric> format
		Name: *azVM.Name,  // this will later get overwritten with azure VM tag stored in ['Name']
		Location: &models.VirtualMachineLocation{
			Region: *azVM.Location,
		},
		Template: &models.VirtualMachineTemplate{},
		Tags:     map[string]*string{},
	}

	if azVM.Properties != nil {
		provState := strings.ToLower(*azVM.Properties.ProvisioningState)
		var powerState string

		if azVM.Properties.InstanceView != nil {
			for _, status := range azVM.Properties.InstanceView.Statuses {
				if strings.Contains(*status.Code, "PowerState") {
					statusParts := strings.Split(*status.Code, "/")
					powerState = strings.ToLower(statusParts[1])
				}
			}
		}

		// map the provisioning and power state into a single cloudy state
		switch provState {
		case CREATING:
			cloudyVm.State = CREATING
		case DELETING:
			cloudyVm.State = DELETING
		case "succeeded":
			switch powerState {
			case RUNNING:
				cloudyVm.State = RUNNING
			case STOPPED:
				cloudyVm.State = STOPPED
			case "deallocated": 
				cloudyVm.State = STOPPED  // TODO: Should VM ever be left in deallocated state?
			default:
				cloudyVm.State = UNKNOWN
			}
		case UPDATING:
			switch powerState {
			case STARTING:
				cloudyVm.State = STARTING
			case "deallocating":
				cloudyVm.State = STOPPING
			case STOPPING:
				cloudyVm.State = STOPPING
			case RESTARTING:
				cloudyVm.State = RESTARTING
			default:
				cloudyVm.State = UNKNOWN
			}
		case FAILED:
			cloudyVm.State = FAILED
		default:
			cloudyVm.State = UNKNOWN
		}
		
		if cloudyVm.State == UNKNOWN {
			log.WarnContext(ctx, fmt.Sprintf("Found %s VM state for VMID:[%s]", UNKNOWN, cloudyVm.ID))
		}

		log.DebugContext(ctx, fmt.Sprintf("Azure ToCloudyVirtualMachine: VMID:[%s] provState:[%s] powerState:[%s] >> cloudy state:[%s]", 
			cloudyVm.ID, provState, powerState, cloudyVm.State))

		if azVM.Properties.NetworkProfile != nil {
			nics := []*models.VirtualMachineNic{}
			for _, nic := range azVM.Properties.NetworkProfile.NetworkInterfaces {
				nics = append(nics, &models.VirtualMachineNic{ID: *nic.ID})
			}
			cloudyVm.Nics = nics
		}

		if azVM.Properties.StorageProfile != nil {
			if azVM.Properties.StorageProfile.OSDisk != nil {
				cloudyVm.Template.OperatingSystem = string(*azVM.Properties.StorageProfile.OSDisk.OSType)

				if azVM.Properties.StorageProfile.OSDisk.ManagedDisk != nil &&
					azVM.Properties.StorageProfile.OSDisk.ManagedDisk.ID != nil {
					cloudyVm.OsDisk = &models.VirtualMachineDisk{
						ID:     *azVM.Properties.StorageProfile.OSDisk.ManagedDisk.ID,
						OsDisk: true,
					}

					if azVM.Properties.StorageProfile.OSDisk.DiskSizeGB != nil {
						cloudyVm.OsDisk.Size = int64(*azVM.Properties.StorageProfile.OSDisk.DiskSizeGB)
					}
				}
			}

			if azVM.Properties.StorageProfile.DataDisks != nil {
				disks := []*models.VirtualMachineDisk{}
				for _, disk := range azVM.Properties.StorageProfile.DataDisks {
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

	if azVM.Tags != nil {
		for k, v := range azVM.Tags {
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

func LongIdToShortId(longId string) string {
	parts := strings.Split(longId, "/")
	return parts[len(parts)-1]
}
