package cloudyazure

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/appliedres/cloudy/models"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
)

func (vmm *AzureVirtualMachineManager) FindBestAzureVMSize(ctx context.Context, location string, template models.VirtualMachineTemplate) (*armcompute.ResourceSKU, []*armcompute.ResourceSKU, error) {
	pager := vmm.sizesClient.NewListPager(nil)
	var skus []*armcompute.ResourceSKU
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, nil, err
		}
		skus = append(skus, page.Value...)
	}

	// Filter SKUs by location and resource type
	filteredSkus := filterSkusByLocationAndType(skus, location, "virtualMachines")

	// Score SKUs and find the best match
	var bestMatch *armcompute.ResourceSKU
	var nearMatches []*armcompute.ResourceSKU
	highestScore := 0.0

	for _, sku := range filteredSkus {
		score := calculateScore(template, sku)
		if score > highestScore {
			highestScore = score
			bestMatch = sku
		}
		if score > 0.8 { // Threshold for near matches
			nearMatches = append(nearMatches, sku)
		}
	}

	if bestMatch == nil {
		return nil, nearMatches, errors.New("no suitable VM size found")
	}

	return bestMatch, nearMatches, nil
}

func filterSkusByLocationAndType(skus []*armcompute.ResourceSKU, location, resourceType string) []*armcompute.ResourceSKU {
	var filtered []*armcompute.ResourceSKU
	for _, sku := range skus {
		// Check if sku has valid locations and resource type
		if sku.Locations == nil || sku.ResourceType == nil || *sku.ResourceType != resourceType {
			continue
		}

		// Check if the location exists in the Locations slice
		found := false
		for _, loc := range sku.Locations {
			if loc != nil && *loc == location {
				found = true
				break
			}
		}
		if !found {
			continue
		}

		filtered = append(filtered, sku)
	}
	return filtered
}

func calculateScore(template models.VirtualMachineTemplate, sku *armcompute.ResourceSKU) float64 {
	if sku.Capabilities == nil {
		return 0
	}

	var cpuCores, ram, gpuCount, maxNics float64
	for _, capability := range sku.Capabilities {
		if capability.Name == nil || capability.Value == nil {
			continue
		}

		switch strings.ToLower(*capability.Name) {
		case "vcpus":
			if value, err := strconv.ParseFloat(*capability.Value, 64); err == nil {
				cpuCores = value
			}
		case "memorygb":
			if value, err := strconv.ParseFloat(*capability.Value, 64); err == nil {
				ram = value
			}
		case "gpu":
			if value, err := strconv.ParseFloat(*capability.Value, 64); err == nil {
				gpuCount = value
			}
		case "maxnics":
			if value, err := strconv.ParseFloat(*capability.Value, 64); err == nil {
				maxNics = value
			}
		}
	}

	return calculateDynamicScore(float64(template.MinCPU), float64(template.MaxCPU), cpuCores) *
		calculateDynamicScore(template.MinRAM, template.MaxRAM, ram) *
		calculateDynamicScore(float64(template.MinGpu), float64(template.MaxGpu), gpuCount) *
		calculateDynamicScore(float64(template.MinNic), float64(template.MaxNic), maxNics)
}

func calculateDynamicScore(min, max, actual float64) float64 {
	if actual < min {
		return actual / min
	}
	if actual > max {
		return max / actual
	}
	return 1
}
