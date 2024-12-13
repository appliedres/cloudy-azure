package cloudyazure

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
)

func (vmm *AzureVirtualMachineManager) GetAllSizes(ctx context.Context) (map[string]*models.VirtualMachineSize, error) {

	sizesList := map[string]*models.VirtualMachineSize{}

	pager := vmm.sizesClient.NewListPager(&armcompute.ResourceSKUsClientListOptions{})
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return sizesList, errors.Wrap(err, "GetSizes.NextPage")
		}

		for _, sku := range resp.Value {
			if strings.EqualFold(*sku.ResourceType, "virtualMachines") && 
				isFamilyValid(*sku.Family) && 
				!isSizeRestricted(sku.Restrictions) {

				existingSize, sizeExists := sizesList[*sku.Name]

				// update if size already exists
				if sizeExists {
					for _, location := range sku.Locations {
						_, locationExists := existingSize.Locations[*location]

						// location not found in size found in sizeslist, add it now
						if !locationExists {
							existingSize.Locations[*location] = ToCloudyVirtualMachineLocation(location)
							sizesList[*sku.Name] = existingSize
						}
					}
				} else {
					sizesList[*sku.Name] = ToCloudyVirtualMachineSize(ctx, sku)
				}
			}
		}
	}

	return sizesList, nil
}

type ScoredVMSize struct {
	Score float64
	Size  *models.VirtualMachineSize
}

func (vmm *AzureVirtualMachineManager) GetSizesForTemplate(ctx context.Context, template models.VirtualMachineTemplate) (
	matches map[string]*models.VirtualMachineSize, 
	worse map[string]*models.VirtualMachineSize, 
	better map[string]*models.VirtualMachineSize,  
	err error) {

	sizes, err := vmm.GetSizesWithUsage(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "could not query all sizes")
	}

	matches = make(map[string]*models.VirtualMachineSize)
	worse = make(map[string]*models.VirtualMachineSize)
	better = make(map[string]*models.VirtualMachineSize)

	for _, size := range sizes {
		if size.Available < 1 {
			continue  // ignore sizes that are not available
		}

		score := scoreVMSize(size, template)

		if score == 1 {
			matches[strconv.Itoa(len(matches))] = size // Perfect match
		} else if score < 0 {
			worse[strconv.Itoa(len(worse))] = size // overall worse performance
		} else {
			better[strconv.Itoa(len(better))] = size // overall better performance, including score == 0
		}
	}

	return matches, worse, better, nil
}

func (vmm *AzureVirtualMachineManager) GetUsage(ctx context.Context) (map[string]models.VirtualMachineFamily, error) {

	usageList := map[string]models.VirtualMachineFamily{}

	pager := vmm.usageClient.NewListPager(vmm.credentials.Location, &armcompute.UsageClientListOptions{})

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return usageList, errors.Wrap(err, "GetUsage.NextPage")
		}

		for _, v := range resp.Value {
			if isFamilyValid(*v.Name.Value) {
				family := models.VirtualMachineFamily{
					Name:  *v.Name.Value,
					Usage: int64(*v.CurrentValue),
					Quota: *v.Limit,
				}

				usageList[family.Name] = family
			}

		}

	}

	return usageList, nil
}

func (vmm *AzureVirtualMachineManager) GetSizesWithUsage(ctx context.Context) (map[string]*models.VirtualMachineSize, error) {

	log := logging.GetLogger(ctx)

	sizes, err := vmm.GetAllSizes(ctx)
	if err != nil {
		return nil, err
	}

	usage, err := vmm.GetUsage(ctx)
	if err != nil {
		return nil, err
	}

	for sizeName, size := range sizes {
		u, ok := usage[size.Family.ID]

		if !ok {
			log.WarnContext(ctx, fmt.Sprintf("size %s family %s missing in usage", size.ID, size.Family.ID))
		} else {
			size.Family.Usage = u.Usage
			size.Family.Quota = u.Quota

			available := u.Quota - u.Usage
			size.Available = max(available, 0)  

			sizes[sizeName] = size
		}
	}

	return sizes, nil
}

func isFamilyValid(name string) bool {
	lowerName := strings.ToLower(name)

	if strings.Contains(lowerName, "family") && !strings.Contains(lowerName, "promo") {
		return true
	}

	return false
}

func isSizeRestricted(restrictions []*armcompute.ResourceSKURestrictions) bool {

	restricted := string(armcompute.ResourceSKURestrictionsReasonCodeNotAvailableForSubscription)

	for _, restriction := range restrictions {

		if strings.EqualFold(restricted, string(*restriction.ReasonCode)) {
			return true
		}
	}

	return false
}
func scoreVMSize(size *models.VirtualMachineSize, template models.VirtualMachineTemplate) float64 {
    var cpuScore, ramScore, gpuScore, totalScore float64

    // CPU score
    if template.MinCPU != nil && size.CPU < *template.MinCPU {
        cpuScore = -1 + (float64(size.CPU) / float64(*template.MinCPU)) // worse
    } else if template.MaxCPU != nil && size.CPU > *template.MaxCPU {
        cpuScore = 1 - (float64(*template.MaxCPU) / float64(size.CPU)) // better
    } else {
        cpuScore = 0.0 // Perfect match
    }

    // RAM score
    if template.MinRAM != nil && size.RAM < *template.MinRAM {
        ramScore = -1 + (float64(size.RAM) / float64(*template.MinRAM)) // worse
    } else if template.MaxRAM != nil && size.RAM > *template.MaxRAM {
        ramScore = 1 - (float64(*template.MaxRAM) / float64(size.RAM)) // better
    } else {
        ramScore = 0.0 // Perfect match
    }

    // GPU score
    if template.MinGpu != nil && size.Gpu < *template.MinGpu {
        gpuScore = -1 + (float64(size.Gpu) / float64(*template.MinGpu)) // worse
    } else if template.MaxGpu != nil && size.Gpu > *template.MaxGpu {
        gpuScore = 1 - (float64(*template.MaxGpu) / float64(size.Gpu)) // better
    } else {
        gpuScore = 0.0 // Perfect match
    }

    if cpuScore == 0 &&
        ramScore == 0 &&
        gpuScore == 0 {
        return 1 // exact match
    }
    
    // Calculate final score. 
    // The closer to 0, the better the match. A value above 0 indicates better performance, 
    // and below 0 indicates worse performance.
	totalScore = (cpuScore + ramScore + gpuScore) / 3
    return totalScore
}

// This function was unused in v1
// It would need the config to have the source image gallery name in it
func (vmm *AzureVirtualMachineManager) GetLatestImageVersion(ctx context.Context, imageName string) (string, error) {

	log := logging.GetLogger(ctx)

	// set this for real if we're going to use this function
	sourceImageGalleryName := ""

	pager := vmm.galleryClient.NewListPager(vmm.credentials.Region, sourceImageGalleryName, imageName, &armcompute.SharedGalleryImageVersionsClientListOptions{})

	var allVersions []*version.Version

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return "", errors.Wrap(err, "GetLatestImageVersion")
		}
		for _, imageVersion := range resp.Value {
			v, err := version.NewVersion(*imageVersion.Name)
			if err != nil {
				log.ErrorContext(ctx, fmt.Sprintf("Skipping Invalid Version : %v", *imageVersion.Name), logging.WithError(err))
				continue
			}
			allVersions = append(allVersions, v)
		}
	}

	sort.Sort(version.Collection(allVersions))

	latest := allVersions[len(allVersions)-1]

	return latest.Original(), nil
}
