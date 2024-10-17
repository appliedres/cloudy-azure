package cloudyazure

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
)

func (vmm *AzureVirtualMachineManager) GetData(ctx context.Context) (map[string]models.VirtualMachineSize, error) {

	dataList := map[string]models.VirtualMachineSize{}

	pager := vmm.dataClient.NewListPager(&armcompute.ResourceSKUsClientListOptions{})
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return dataList, errors.Wrap(err, "GetData.NextPage")
		}

		for _, v := range resp.Value {
			if strings.EqualFold(*v.ResourceType, "virtualMachines") && isFamilyValid(*v.Family) && isSizeAvailable(v.Restrictions) {

				mapSizeData, sizeOk := dataList[*v.Name]

				// size found in datalist
				if sizeOk {
					for _, location := range v.Locations {
						_, locationOk := mapSizeData.Locations[*location]

						// location not found in size found in datalist
						if !locationOk {
							mapSizeData.Locations[*location] = ToCloudyVirtualMachineLocation(location)
							dataList[*v.Name] = mapSizeData
						}
					}

				} else {
					dataList[*v.Name] = *ToCloudyVirtualMachineSize(ctx, v)
				}

			}
		}
	}

	return dataList, nil
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

func (vmm *AzureVirtualMachineManager) GetDataWithUsage(ctx context.Context) (map[string]models.VirtualMachineSize, error) {

	log := logging.GetLogger(ctx)

	sizes, err := vmm.GetData(ctx)
	if err != nil {
		return nil, err
	} else {
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

				sizes[sizeName] = size
			}
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

func isSizeAvailable(restrictions []*armcompute.ResourceSKURestrictions) bool {

	notAvailable := string(armcompute.ResourceSKURestrictionsReasonCodeNotAvailableForSubscription)

	for _, restriction := range restrictions {

		if strings.EqualFold(notAvailable, string(*restriction.ReasonCode)) {
			return false
		}
	}

	return true
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
