package vm

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
)

const (
	vmNameTagKey    = "Name"
	vmCreatorTagKey = "CreatorID"
	vmUserTagKey    = "UserID"
	vmTeamTagKey    = "TeamID"
)

func FromCloudyVirtualMachine(ctx context.Context, cloudyVM *models.VirtualMachine) (*armcompute.VirtualMachine, error) {
	// log := logging.GetLogger(ctx)

	azVM := armcompute.VirtualMachine{
		// cloudyVM Id is saved as ID and Name
		// cloudyVM Name is saved in a Tag
		ID:       &cloudyVM.ID,
		Name:     &cloudyVM.ID,
		Location: &cloudyVM.Location.Region,
		Identity: &armcompute.VirtualMachineIdentity{
			Type: to.Ptr(armcompute.ResourceIdentityTypeSystemAssigned),
		},
	}

	if cloudyVM.Tags != nil {
		azVM.Tags = cloudyVM.Tags
	} else {
		azVM.Tags = make(map[string]*string)
	}

	azVM.Tags[vmNameTagKey] = &cloudyVM.Name

	// Add creator, user, and team tags
	if cloudyVM.CreatorID != "" {
		azVM.Tags[vmCreatorTagKey] = &cloudyVM.CreatorID
	}
	if cloudyVM.UserID != "" {
		azVM.Tags[vmUserTagKey] = &cloudyVM.UserID
	}
	if cloudyVM.TeamID != "" {
		azVM.Tags[vmTeamTagKey] = &cloudyVM.TeamID
	}

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

	imgRef, imgPlan, err := translateImageID(ctx, cloudyVM.Template.OsBaseImageID)
	if err != nil {
		return nil, fmt.Errorf("VM image ID parsing failed: %w", err)
	}

	// Set Marketplace Plan if it is specified
	if imgPlan != nil {
		azVM.Plan = imgPlan
	}

	azVM.Properties = &armcompute.VirtualMachineProperties{
		HardwareProfile: &armcompute.HardwareProfile{
			VMSize: (*armcompute.VirtualMachineSizeTypes)(&cloudyVM.Template.Size.ID),
		},
		StorageProfile: &armcompute.StorageProfile{
			ImageReference: imgRef,
			OSDisk: &armcompute.OSDisk{
				CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
			},
		},
	}

	if cloudyVM.Template.SecurityProfile != nil {
		azVM.Properties.SecurityProfile = &armcompute.SecurityProfile{}

		switch cloudyVM.Template.SecurityProfile.SecurityTypes {
		case models.VirtualMachineSecurityTypesConfidentialVM:
			azVM.Properties.SecurityProfile.SecurityType = to.Ptr(armcompute.SecurityTypesConfidentialVM)
		case models.VirtualMachineSecurityTypesTrustedLaunch:
			azVM.Properties.SecurityProfile.SecurityType = to.Ptr(armcompute.SecurityTypesTrustedLaunch)
		}
	}

	azVM.Properties.OSProfile = &armcompute.OSProfile{
		ComputerName:  to.Ptr(cloudyVM.ID),
		AdminUsername: &cloudyVM.Template.LocalAdministratorID,
		AdminPassword: to.Ptr(cloudy.GeneratePassword(15, 2, 2, 2)),
	}

	// OS-specific items
	switch cloudyVM.Template.OperatingSystem {
	case "windows":
		azVM.Properties.StorageProfile.OSDisk.OSType = to.Ptr(armcompute.OperatingSystemTypesWindows)
		azVM.Properties.OSProfile.WindowsConfiguration = &armcompute.WindowsConfiguration{}
	case "linux":
		azVM.Properties.StorageProfile.OSDisk.OSType = to.Ptr(armcompute.OperatingSystemTypesLinux)
		azVM.Properties.OSProfile.LinuxConfiguration = &armcompute.LinuxConfiguration{
			DisablePasswordAuthentication: to.Ptr(false),
			// TODO: linux SSH key management
			// SSH: &armcompute.SSHConfiguration{
			// 	PublicKeys: []*armcompute.SSHPublicKey{
			// 		{
			// 			Path:    to.Ptr(fmt.Sprintf("/home/%s/.ssh/authorized_keys", cloudyVM.Template.LocalAdministratorID)),
			// 			KeyData: to.Ptr(),
			// 		},
			// 	},
			// },
			ProvisionVMAgent: to.Ptr(true),
		}
		azVM.Properties.OSProfile.AllowExtensionOperations = to.Ptr(true)
	}

	// NICs
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

	return &azVM, nil
}

func ToCloudyVirtualMachine(ctx context.Context, azVM *armcompute.VirtualMachine) *models.VirtualMachine {
	cloudyVm := models.VirtualMachine{
		ID:   *azVM.Name, // Azure VM Name is cloudy ID, in UVM-<alphanumeric> format
		Name: *azVM.Name, // this will later get overwritten with azure VM tag stored in ['Name']
		Location: &models.VirtualMachineLocation{
			Region: *azVM.Location,
		},
		Template: &models.VirtualMachineTemplate{},
		Tags:     map[string]*string{},
	}

	if azVM.Properties != nil {
		cloudyVm.CloudState = mapProvisioningAndPowerState(ctx, azVM)

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

		if azVM.Properties.SecurityProfile != nil {
			cloudyVm.Template.SecurityProfile = &models.VirtualMachineSecurityProfileConfiguration{}

			if azVM.Properties.SecurityProfile.SecurityType != nil {

				switch *azVM.Properties.SecurityProfile.SecurityType {
				case armcompute.SecurityTypesConfidentialVM:
					cloudyVm.Template.SecurityProfile.SecurityTypes = models.VirtualMachineSecurityTypesConfidentialVM
				case armcompute.SecurityTypesTrustedLaunch:
					cloudyVm.Template.SecurityProfile.SecurityTypes = models.VirtualMachineSecurityTypesTrustedLaunch
				}
			}
		}
	}

	if azVM.Tags != nil {
		for k, v := range azVM.Tags {
			if strings.EqualFold(k, vmNameTagKey) {
				cloudyVm.Name = *v
			} else if strings.EqualFold(k, vmCreatorTagKey) {
				cloudyVm.CreatorID = *v
			} else if strings.EqualFold(k, vmUserTagKey) {
				cloudyVm.UserID = *v
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

// Finds and converts Azure's ProvisioningState and PowerState into a cloudy CloudState
func mapProvisioningAndPowerState(ctx context.Context, azVM *armcompute.VirtualMachine) *models.VirtualMachineCloudState {
	log := logging.GetLogger(ctx)

	provState := strings.ToLower(*azVM.Properties.ProvisioningState)
	var powerState string

	if azVM.Properties.InstanceView == nil {
		return nil // no InstanceView, we likely did not query with IncludeState
	}

	for _, status := range azVM.Properties.InstanceView.Statuses {
		if strings.Contains(*status.Code, "PowerState") {
			statusParts := strings.Split(*status.Code, "/")
			powerState = strings.ToLower(statusParts[1])
			break
		}
	}

	cloudState := mapCloudState(provState, powerState)

	if cloudState == models.VirtualMachineCloudStateUnknown {
		log.WarnContext(ctx, fmt.Sprintf("Found %s VM state for VMID:[%s]", models.VirtualMachineCloudStateUnknown, *azVM.ID))
	}

	log.DebugContext(ctx, fmt.Sprintf("Azure ToCloudyVirtualMachine: VMID:[%s] provState:[%s] powerState:[%s] >> cloudy state:[%s]",
		*azVM.Name, provState, powerState, cloudState))

	return &cloudState
}

// Maps to a single cloudy CloudyState from Azure's combination of provisioning and power states
func mapCloudState(provState, powerState string) models.VirtualMachineCloudState {
	switch provState {
	case string(models.VirtualMachineCloudStateCreating):
		return models.VirtualMachineCloudStateCreating
	case string(models.VirtualMachineCloudStateDeleting):
		return models.VirtualMachineCloudStateDeleting
	case "succeeded":
		switch powerState {
		case string(models.VirtualMachineCloudStateRunning):
			return models.VirtualMachineCloudStateRunning
		case string(models.VirtualMachineCloudStateStopped):
			return models.VirtualMachineCloudStateStopped
		case "deallocated":
			return models.VirtualMachineCloudStateDeleted // TODO: Should VM ever be left in deallocated state?
		default:
			return models.VirtualMachineCloudStateUnknown
		}
	case "updating":
		switch powerState {
		case string(models.VirtualMachineCloudStateStarting):
			return models.VirtualMachineCloudStateStarting
		case "deallocating":
			return models.VirtualMachineCloudStateDeleting
		case string(models.VirtualMachineCloudStateStopping):
			return models.VirtualMachineCloudStateStopping
		case string(models.VirtualMachineCloudStateRestarting):
			return models.VirtualMachineCloudStateRestarting
		case string(models.VirtualMachineCloudStateRunning): // 'updating' a 'running' VM is 'stopping'
			return models.VirtualMachineCloudStateStopping
		case string(models.VirtualMachineCloudStateStopped): // 'updating' a 'stopped' VM is 'starting'
			return models.VirtualMachineCloudStateStarting
		default:
			return models.VirtualMachineCloudStateUnknown
		}
	case string(models.VirtualMachineCloudStateFailed):
		return models.VirtualMachineCloudStateFailed
	default:
		return models.VirtualMachineCloudStateUnknown
	}
}

const mpPrefix = "marketplace::" // shorthand for marketplace

// translateImageID builds the ImageReference and optional Plan from the encoded ID.
// Format:
//   marketplace:
//		"marketplace::<Publisher>::<Offer>::<SKU>::<Version>[::PlanName]"
//   gallery: 
//		"/subscriptions/<SubscriptionID/resourceGroups/<ResourceGroup>/providers/Microsoft.Compute/galleries/<ImageGalleryName/images/<ImageName>/versions/<version>"
func translateImageID(ctx context.Context, imageID string) (*armcompute.ImageReference, *armcompute.Plan, error) {
	log := logging.GetLogger(ctx)

	// Gallery image
	if !strings.HasPrefix(imageID, mpPrefix) {
		log.DebugContext(ctx, fmt.Sprintf("Detected gallery image ID: %q", imageID))
		return &armcompute.ImageReference{ID: to.Ptr(imageID)}, nil, nil
	}

	// Marketplace image
	log.DebugContext(ctx, fmt.Sprintf("Translating marketplace image ID: %q", imageID))
	parts := strings.Split(strings.TrimPrefix(imageID, mpPrefix), "::")
	log.DebugContext(ctx, fmt.Sprintf("Marketplace image parts: %v", parts))

	if len(parts) < 4 || len(parts) > 5 {
		err := fmt.Errorf("invalid marketplace image spec: must be marketplace::<Publisher>::<Offer>::<SKU>::<Version>[::PlanName], got %q", imageID)
		log.ErrorContext(ctx, "Error parsing marketplace image ID", logging.WithError(err))
		return nil, nil, err
	}

	publisher := parts[0]
	offer := parts[1]
	sku := parts[2]
	version := parts[3]

	log.InfoContext(ctx, fmt.Sprintf("Using marketplace image: Publisher=%s, Offer=%s, SKU=%s, Version=%s", publisher, offer, sku, version))

	img := &armcompute.ImageReference{
		Publisher: to.Ptr(publisher),
		Offer:     to.Ptr(offer),
		SKU:       to.Ptr(sku),
		Version:   to.Ptr(version),
	}

	var plan *armcompute.Plan
	if len(parts) == 5 && parts[4] != "" {
		plan = &armcompute.Plan{
			Name:      to.Ptr(parts[4]),
			Publisher: to.Ptr(publisher),
			Product:   to.Ptr(offer),
		}
		log.InfoContext(ctx, fmt.Sprintf("Using image plan: Name=%s, Publisher=%s, Product=%s", parts[4], publisher, offer))
	}

	return img, plan, nil
}
