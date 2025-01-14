package avd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy"
)

func (avd *AzureVirtualDesktopManager) FindFirstAvailableHostPool(ctx context.Context, rgName string, upn string) (*armdesktopvirtualization.HostPool, error) {
	// Get all the host pools
	all, err := avd.listHostPools(ctx, rgName, nil)
	if err != nil {
		return nil, err
	}

	for _, hostpool := range all {
		// List all the sessions for a given host pool
		sessions, err := avd.listSessionHosts(ctx, rgName, *hostpool.Name)
		if err != nil {
			return nil, err
		}

		foundUser := false
		for _, session := range sessions {
			assigned := session.Properties.AssignedUser
			if assigned != nil && *assigned == upn {
				foundUser = true
				break
			}
		}

		if !foundUser {
			return hostpool, nil
		}
	}

	return nil, nil
}

func (avd *AzureVirtualDesktopManager) RetrieveRegistrationToken(ctx context.Context, rgName string, hpname string) (*string, error) {
	tokenresponse, err := avd.hostPoolsClient.RetrieveRegistrationToken(ctx, rgName, hpname, nil)
	if tokenresponse.Token == nil || err != nil {
		return nil, cloudy.Error(ctx, "RetrieveRegistrationToken avd.hostPoolsClient.RetrieveRegistrationToken failure: %+v", err)
	}

	// token has expired
	if tokenresponse.ExpirationTime.Before(time.Now()) {
		hp, err := avd.UpdateHostPoolRegToken(ctx, rgName, hpname)
		if hp == nil || err != nil {
			return nil, cloudy.Error(ctx, "RetrieveRegistrationToken avd.UpdateHostPoolRegToken failure: %+v", err)
		}

		time.Sleep(3 * time.Second)

		tokenresponse, err := avd.hostPoolsClient.RetrieveRegistrationToken(ctx, rgName, hpname, nil)
		if tokenresponse.Token == nil || err != nil {
			return nil, cloudy.Error(ctx, "RetrieveRegistrationToken avd.hostPoolsClient.RetrieveRegistrationToken failure: %+v", err)
		}
	}

	return tokenresponse.Token, err
}

// Helper to check if a host pool is empty
func (avd *AzureVirtualDesktopManager) isHostPoolEmpty(ctx context.Context, rgName, hostPoolName string) (bool, error) {
	sessionHostsPager := avd.sessionHostsClient.NewListPager(rgName, hostPoolName, nil)
	for sessionHostsPager.More() {
		page, err := sessionHostsPager.NextPage(ctx)
		if err != nil {
			return false, fmt.Errorf("error listing session hosts in host pool %s: %w", hostPoolName, err)
		}
		if len(page.Value) > 0 {
			return false, nil
		}
	}
	return true, nil
}

// CreateHostPool creates a new host pool.
func (avd *AzureVirtualDesktopManager) CreateHostPool(ctx context.Context, rgName, suffix string, tags map[string]*string) (*armdesktopvirtualization.HostPool, error) {
	hostPoolName := avd.config.HostPoolNamePrefix + suffix

	// Expiration time can be 1 hour to 27 days. We'll use 25 days.
	expirationTime := time.Now().AddDate(0, 0, 25) // 25 days from now

	newHostPool := armdesktopvirtualization.HostPool{
		Location: to.Ptr(string(avd.credentials.Region)),
		Tags:     tags,
		Properties: &armdesktopvirtualization.HostPoolProperties{
			FriendlyName: to.Ptr("Host Pool " + suffix),
			Description:  to.Ptr("Generated via cloudy-azure"),
			HostPoolType: to.Ptr(armdesktopvirtualization.HostPoolTypePersonal),
			RegistrationInfo: &armdesktopvirtualization.RegistrationInfo{
				ExpirationTime:             &expirationTime,
				RegistrationTokenOperation: to.Ptr(armdesktopvirtualization.RegistrationTokenOperationUpdate),
			},
		},
	}

	resp, err := avd.hostPoolsClient.CreateOrUpdate(ctx, rgName, hostPoolName, newHostPool, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create host pool: %w", err)
	}

	return &resp.HostPool, nil
}

// CanAssignUserToHostPool checks if the specified user is already assigned to a session host in the given host pool.
func (avd *AzureVirtualDesktopManager) CanAssignUserToHostPool(ctx context.Context, rgName, hostPoolName, userName string) (bool, error) {
	pager := avd.sessionHostsClient.NewListPager(rgName, hostPoolName, nil)

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to list session hosts: %w", err)
		}

		for _, sessionHost := range page.Value {
			if sessionHost.Properties != nil && sessionHost.Properties.AssignedUser != nil {
				if strings.EqualFold(*sessionHost.Properties.AssignedUser, userName) {
					return false, nil // User is already assigned to this session host
				}
			}
		}
	}

	return true, nil // User is not assigned to any session host in the host pool
}

func (avd *AzureVirtualDesktopManager) UpdateHostPoolRegToken(ctx context.Context, rgName string, hpName string) (*armdesktopvirtualization.HostPool, error) {
	// Expiration time can be 1 hour to 27 days. We'll use 25 days.
	expirationTime := time.Now().AddDate(0, 0, 25) // 25 days from now

	patch := armdesktopvirtualization.HostPoolPatch{
		Properties: &armdesktopvirtualization.HostPoolPatchProperties{
			RegistrationInfo: &armdesktopvirtualization.RegistrationInfoPatch{
				ExpirationTime:             &expirationTime,
				RegistrationTokenOperation: to.Ptr(armdesktopvirtualization.RegistrationTokenOperationUpdate),
			},
		}}

	resp, err := avd.hostPoolsClient.Update(ctx, rgName, hpName, &armdesktopvirtualization.HostPoolsClientUpdateOptions{
		HostPool: &patch,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update host pool reg token: %w", err)
	}

	return &resp.HostPool, nil
}

func (avd *AzureVirtualDesktopManager) DeleteHostPool(ctx context.Context, rgName string, hpName string) error {
	_, err := avd.hostPoolsClient.Delete(ctx, rgName, hpName, nil)
	if err != nil {
		return fmt.Errorf("failed to update host pool reg token: %w", err)
	}

	return nil
}

func (avd *AzureVirtualDesktopManager) listHostPools(ctx context.Context, rgName string, prefixFilter *string) ([]*armdesktopvirtualization.HostPool, error) {
	pager := avd.hostPoolsClient.NewListByResourceGroupPager(rgName, &armdesktopvirtualization.HostPoolsClientListByResourceGroupOptions{})
	var all []*armdesktopvirtualization.HostPool

	for {
		if !pager.More() {
			break
		}
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		// Filter host pools by prefixFilter
		for _, pool := range resp.HostPoolList.Value {
			if prefixFilter == nil || strings.HasPrefix(*pool.Name, *prefixFilter) {
				all = append(all, pool)
			}
		}
	}

	return all, nil
}
