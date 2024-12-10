package cloudyazure

import (
	"context"
	"fmt"
	"math"
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

func (vmm *AzureVirtualMachineManager) GetSizesForTemplate(ctx context.Context, template models.VirtualMachineTemplate) (map[string]*models.VirtualMachineSize, error) {
	// Step 1: Get all available sizes
	sizes, err := vmm.GetSizesWithUsage(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not query all sizes")
	}

	// Step 2: Score all sizes
	scoredSizes := []ScoredVMSize{}
	for _, size := range sizes {

		if size.Available < 1{
			continue
		}

		score := scoreVMSize(size, template)
		if score == 1 {  // TODO: for now only include exact matches
			scoredSizes = append(scoredSizes, ScoredVMSize{
				Score: score,
				Size:  size,
			})
		}
	}

	// Step 3: Sort the scored sizes
    // sort.Slice(scoredSizes, func(i, j int) bool {
    //     // Exact match first (highest score = 1.0)
    //     if scoredSizes[i].Score == 1.0 && scoredSizes[j].Score != 1.0 {
    //         return true
    //     }
    //     if scoredSizes[i].Score != 1.0 && scoredSizes[j].Score == 1.0 {
    //         return false
    //     }

    //     // For all other cases, prioritize higher scores
    //     return scoredSizes[i].Score > scoredSizes[j].Score
    // })

	// Step 4: Rank the sizes and assign them to a map by rank as string
	rankedMap := make(map[string]*models.VirtualMachineSize)
	for rank, scoredSize := range scoredSizes {
		rankedMap[strconv.Itoa(rank)] = scoredSize.Size
	}

	return rankedMap, nil
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
    score := 0.0
    isExactMatch := true

    // CPU score
    cpuScore := 0.0
    if template.MinCPU > 0 && size.CPU < template.MinCPU {
        isExactMatch = false
        cpuScore = float64(size.CPU) / float64(template.MinCPU)
    } else if template.MaxCPU > 0 && size.CPU > template.MaxCPU {
        isExactMatch = false
        cpuScore = float64(template.MaxCPU) / float64(size.CPU)
    } else if template.MinCPU > 0 && template.MaxCPU > 0 {
        cpuScore = 1.0 - math.Abs(float64(size.CPU-template.MinCPU))/float64(template.MaxCPU-template.MinCPU)
    } else {
        cpuScore = 1.0
    }

    // RAM score
    ramScore := 0.0
    if template.MinRAM > 0 && size.RAM < template.MinRAM {
        isExactMatch = false
        ramScore = float64(size.RAM) / float64(template.MinRAM)
    } else if template.MaxRAM > 0 && size.RAM > template.MaxRAM {
        isExactMatch = false
        ramScore = float64(template.MaxRAM) / float64(size.RAM)
    } else if template.MinRAM > 0 && template.MaxRAM > 0 {
        ramScore = 1.0 - math.Abs(float64(size.RAM-template.MinRAM))/float64(template.MaxRAM-template.MinRAM)
    } else {
        ramScore = 1.0
    }

    // GPU score
    gpuScore := 0.0
    if template.MinGpu > 0 && size.Gpu < template.MinGpu {
        isExactMatch = false
        gpuScore = float64(size.Gpu) / float64(template.MinGpu)
    } else if template.MaxGpu > 0 && size.Gpu > template.MaxGpu {
        isExactMatch = false
        gpuScore = float64(template.MaxGpu) / float64(size.Gpu)
    } else if template.MinGpu > 0 && template.MaxGpu > 0 {
        gpuScore = 1.0 - math.Abs(float64(size.Gpu-template.MinGpu))/float64(template.MaxGpu-template.MinGpu)
    } else {
        gpuScore = 1.0
    }

    if isExactMatch {
        return 1.0
    }

    score = (cpuScore + ramScore + gpuScore) / 3.0
    return score
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
