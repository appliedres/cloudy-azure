package avd

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy/logging"
)

// Validates that we have a valid AVD stack for RemoteApps.
func (avd *AzureVirtualDesktopManager) EnsurePooledStack(
	ctx context.Context,
	loadBalancerType armdesktopvirtualization.LoadBalancerType,
	maxSessionLimit int32,
) (*armdesktopvirtualization.HostPool, *armdesktopvirtualization.Workspace, error) {

	suffix := avd.Name

	tags := map[string]*string{
		"suffix":             to.Ptr(suffix),
		"arkloud_created_by": to.Ptr("cloudy-azure"),
	}

	log := logging.GetLogger(ctx)
	log.InfoContext(ctx, "Ensuring pooled stack exists", "HostPoolName", avd.Config.PooledHostPoolNamePrefix+suffix)

	// Ensure host pool
	hostPool, err := avd.EnsurePooledHostPoolForRemoteApps(ctx, suffix, loadBalancerType, maxSessionLimit, tags)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to ensure host pool: %w", err)
	}

	// Ensure workspace
	workspace, err := avd.EnsureWorkspace(ctx, suffix, tags)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to ensure workspace: %w", err)
	}

	// TODO: ensureCapacity here?

	return hostPool, workspace, nil
}

// EnsurePooledHostPoolForRemoteApps makes sure one host pool of type "Pooled" (RailApplications) exists.
// If an existing host pool is found but doesn't match the expected configuration, we return an error
// (instead of updating it).
func (avd *AzureVirtualDesktopManager) EnsurePooledHostPoolForRemoteApps(
	ctx context.Context,
	suffix string,
	loadBalancerType armdesktopvirtualization.LoadBalancerType,
	maxSessionLimit int32,
	tags map[string]*string,
) (*armdesktopvirtualization.HostPool, error) {
	log := logging.GetLogger(ctx)
	wantedHPName := avd.Config.PooledHostPoolNamePrefix + suffix
	log.InfoContext(ctx, "Ensuring pooled (RemoteApps) host pool exists", "HostPoolName", wantedHPName)

	// 1. List existing host pools
	all, err := avd.listHostPools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list host pools: %w", err)
	}

	// 2. Check if the desired host pool already exists
	var foundHP *armdesktopvirtualization.HostPool
	for _, hp := range all {
		if hp.Name != nil && *hp.Name == wantedHPName {
			foundHP = hp
			break
		}
	}

	// 3. If not found, create it
	if foundHP == nil {
		log.InfoContext(ctx, "Creating new pooled host pool (RemoteApps)", "Name", wantedHPName)

		expirationTime := time.Now().AddDate(0, 0, 25) // 25 days from now

		newHP := armdesktopvirtualization.HostPool{
			Location: to.Ptr(string(avd.Credentials.Region)),
			Tags:     tags,
			Properties: &armdesktopvirtualization.HostPoolProperties{
				HostPoolType:          to.Ptr(armdesktopvirtualization.HostPoolTypePooled),
				PreferredAppGroupType: to.Ptr(armdesktopvirtualization.PreferredAppGroupTypeRailApplications), // "RailApplications" => RemoteApps
				LoadBalancerType:      to.Ptr(loadBalancerType),
				MaxSessionLimit:       to.Ptr(maxSessionLimit),
				Description:           to.Ptr("Pooled Host Pool for Remote Apps. Managed by cloudy-azure"),
				RegistrationInfo: &armdesktopvirtualization.RegistrationInfo{
					ExpirationTime:             &expirationTime,
					RegistrationTokenOperation: to.Ptr(armdesktopvirtualization.RegistrationTokenOperationUpdate),
				},
			},
		}

		resp, err := avd.hostPoolsClient.CreateOrUpdate(ctx, avd.Credentials.ResourceGroup, wantedHPName, newHP, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create pooled host pool (RemoteApps): %w", err)
		}
		return &resp.HostPool, nil
	}

	// 4. If found, validate required properties.
	//    If something doesn't match, we just return an error (no update attempts).
	if foundHP.Properties == nil {
		return nil, fmt.Errorf("existing host pool %q has nil properties", wantedHPName)
	}

	// Must be Pooled
	if foundHP.Properties.HostPoolType == nil ||
		*foundHP.Properties.HostPoolType != armdesktopvirtualization.HostPoolTypePooled {
		return nil, fmt.Errorf(
			"existing host pool %q is not set to 'Pooled' (found: %v)",
			wantedHPName, foundHP.Properties.HostPoolType,
		)
	}

	// Must be RailApplications
	if foundHP.Properties.PreferredAppGroupType == nil ||
		*foundHP.Properties.PreferredAppGroupType != armdesktopvirtualization.PreferredAppGroupTypeRailApplications {
		return nil, fmt.Errorf(
			"existing host pool %q is not configured for RailApplications (found: %v)",
			wantedHPName, foundHP.Properties.PreferredAppGroupType,
		)
	}

	// Validate load balancer type
	if foundHP.Properties.LoadBalancerType == nil ||
		*foundHP.Properties.LoadBalancerType != loadBalancerType {
		return nil, fmt.Errorf(
			"existing host pool %q has unexpected LoadBalancerType (found: %v, wanted: %v)",
			wantedHPName, foundHP.Properties.LoadBalancerType, loadBalancerType,
		)
	}

	// Validate max session limit
	if foundHP.Properties.MaxSessionLimit == nil ||
		*foundHP.Properties.MaxSessionLimit != maxSessionLimit {
		return nil, fmt.Errorf(
			"existing host pool %q has unexpected MaxSessionLimit (found: %v, wanted: %d)",
			wantedHPName, foundHP.Properties.MaxSessionLimit, maxSessionLimit,
		)
	}

	log.InfoContext(ctx, "Verified existing pooled host pool is configured for RemoteApps", "HostPoolName", wantedHPName)
	return foundHP, nil
}

func (avd *AzureVirtualDesktopManager) EnsureWorkspace(
	ctx context.Context,
	suffix string,
	tags map[string]*string,
) (*armdesktopvirtualization.Workspace, error) {

	log := logging.GetLogger(ctx)
	wantedWSName := avd.Config.PooledWorkspaceNamePrefix + suffix
	log.InfoContext(ctx, "Ensuring workspace exists", "Name", wantedWSName)

	// 1. Check if it already exists
	workspace, err := avd.GetWorkspaceByName(ctx, wantedWSName)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup workspace: %w", err)
	}

	if workspace == nil {
		// 2. Create a new workspace, no linked app groups
		newWS, err := avd.CreatePooledWorkspace(ctx, suffix, tags)
		if err != nil {
			return nil, fmt.Errorf("failed to create workspace %s: %w", wantedWSName, err)
		}
		log.InfoContext(ctx, "Created new workspace", "Name", *newWS.Name)
		return newWS, nil
	}

	// 3. If found, optionally verify some properties or update
	// e.g. check location, tags, etc.
	log.InfoContext(ctx, "Verified existing workspace", "Name", *workspace.Name)
	return workspace, nil
}
