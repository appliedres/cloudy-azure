package avd

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy/logging"
)





func (avd *AzureVirtualDesktopManager) GetDesktopApplicationObjectIDFromAppGroup(ctx context.Context, rgName string, appGroup *armdesktopvirtualization.ApplicationGroup) (string, error) {
	log := logging.GetLogger(ctx)

	if appGroup == nil || appGroup.Name == nil {
		return "", fmt.Errorf("invalid application group provided")
	}

	appPager := avd.appsClient.NewListPager(rgName, *appGroup.Name, nil)
	for appPager.More() {
		page, err := appPager.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to list applications for application group %s: %w", *appGroup.Name, err)
		}
		for _, app := range page.Value {
			if app.Properties != nil && app.Properties.FriendlyName != nil {
				log.InfoContext(ctx, "Found Desktop Application in App Group", "App Group Name", appGroup.Name)
				return *app.ID, nil
			}
		}
	}

	return "", fmt.Errorf("no desktop application found in application group %s", *appGroup.Name)
}

func (avd *AzureVirtualDesktopManager) GetAllDesktopApplications(ctx context.Context, rgName string) ([]string, error) {
	log := logging.GetLogger(ctx)

	appGroupPager := avd.appGroupsClient.NewListByResourceGroupPager(rgName, nil)
	var appIDs []string

	for appGroupPager.More() {
		page, err := appGroupPager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list application groups in resource group %s: %w", rgName, err)
		}
		for _, appGroup := range page.Value {
			if appGroup.Name != nil {
				log.InfoContext(ctx, "Found Application Group", "group_name", *appGroup.Name)

				appPager := avd.appsClient.NewListPager(rgName, *appGroup.Name, nil)
				for appPager.More() {
					appPage, err := appPager.NextPage(ctx)
					if err != nil {
						return nil, fmt.Errorf("failed to list applications for application group %s: %w", *appGroup.Name, err)
					}
					for _, app := range appPage.Value {
						if app.Properties != nil && app.Properties.FriendlyName != nil {
							log.InfoContext(ctx, "Found Desktop Application [%s] with Object ID [%s]", *app.Properties.FriendlyName, *app.ID)
							appIDs = append(appIDs, *app.ID)
						}
					}
				}
			}
		}
	}

	if len(appIDs) == 0 {
		return nil, fmt.Errorf("no desktop applications found in resource group %s", rgName)
	}

	return appIDs, nil
}

func (avd *AzureVirtualDesktopManager) listDesktops(ctx context.Context, rgName string, appGroupName string) ([]*armdesktopvirtualization.Desktop, error) {
	pager := avd.desktopsClient.NewListPager(rgName, appGroupName, nil)
	var allDesktops []*armdesktopvirtualization.Desktop

	for {
		if !pager.More() {
			break
		}

		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		allDesktops = append(allDesktops, resp.Value...)
	}

	return allDesktops, nil
}

func (avd *AzureVirtualDesktopManager) getSingleDesktop(ctx context.Context, rgName string, appGroupName string) (*armdesktopvirtualization.Desktop, error) {
	desktops, err := avd.listDesktops(ctx, rgName, appGroupName)
	if err != nil {
		return nil, fmt.Errorf("failed to list desktops: %w", err)
	}

	if len(desktops) == 0 {
		return nil, fmt.Errorf("no desktops found for appGroupName: %s", appGroupName)
	}
	if len(desktops) > 1 {
		return nil, fmt.Errorf("multiple desktops found for appGroupName: %s", appGroupName)
	}

	return desktops[0], nil
}
