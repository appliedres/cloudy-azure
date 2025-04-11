package avd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy/logging"
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

func (avd *AzureVirtualDesktopManager) RetrieveRegistrationToken(ctx context.Context, rgName, hpName string) (*string, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "Beginning host pool token retrieval")

	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Try to retrieve the token
		tokenResponse, err := avd.hostPoolsClient.RetrieveRegistrationToken(ctx, rgName, hpName, nil)
		if err != nil || tokenResponse.Token == nil {
			log.DebugContext(ctx, fmt.Sprintf("Attempt %d/%d: No valid token found or error retrieving token. Creating/renewing now.",
				attempt, maxRetries,
			))

			// Attempt to create/renew
			hp, updateErr := avd.UpdateHostPoolRegToken(ctx, rgName, hpName)
			if updateErr != nil || hp == nil {
				lastErr = logging.LogAndWrapErr(ctx, log, updateErr, "Failure while creating/renewing host pool token")
				// Let the loop continue and try again
				time.Sleep(3 * time.Second)
				continue
			}

			// Wait briefly, then retrieve again
			time.Sleep(3 * time.Second)
			tokenResponse, err = avd.hostPoolsClient.RetrieveRegistrationToken(ctx, rgName, hpName, nil)
			if err != nil || tokenResponse.Token == nil {
				lastErr = logging.LogAndWrapErr(ctx, log, err, "RetrieveRegistrationToken failure after creating/renewing token")
				time.Sleep(3 * time.Second)
				continue
			}

			log.DebugContext(ctx, "Host pool token has been created/renewed successfully.")
		}

		// Now we have a valid token; check if it expires within 24 hours
		if tokenResponse.ExpirationTime.Before(time.Now().Add(24 * time.Hour)) {
			log.DebugContext(ctx, fmt.Sprintf(
				"Attempt %d/%d: Host pool token will expire within 24 hours. Renewing...",
				attempt, maxRetries,
			))

			hp, updateErr := avd.UpdateHostPoolRegToken(ctx, rgName, hpName)
			if updateErr != nil || hp == nil {
				lastErr = logging.LogAndWrapErr(ctx, log, updateErr, "Failure while renewing host pool token")
				time.Sleep(3 * time.Second)
				continue
			}

			// Wait briefly, then retrieve again
			time.Sleep(3 * time.Second)
			tokenResponse, err = avd.hostPoolsClient.RetrieveRegistrationToken(ctx, rgName, hpName, nil)
			if err != nil || tokenResponse.Token == nil {
				lastErr = logging.LogAndWrapErr(ctx, log, err, "RetrieveRegistrationToken failure after renewing token")
				time.Sleep(3 * time.Second)
				continue
			}

			log.DebugContext(ctx, "Host pool token has been renewed successfully and is valid for at least 24 more hours.")
		}

		// If we reach here, we've successfully retrieved a token that doesn't expire within 24 hours
		log.DebugContext(ctx, "Successfully retrieved host pool token")
		return tokenResponse.Token, nil
	}

	// If we exit the loop, we've used up all retries without success
	if lastErr == nil {
		lastErr = fmt.Errorf("unable to retrieve a valid token after %d attempts", maxRetries)
	}
	return nil, lastErr
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
	hostPoolName := avd.Config.HostPoolNamePrefix + suffix

	// Expiration time can be 1 hour to 27 days. We'll use 25 days.
	expirationTime := time.Now().AddDate(0, 0, 25) // 25 days from now

	newHostPool := armdesktopvirtualization.HostPool{
		Location: to.Ptr(string(avd.Credentials.Region)),
		Tags:     tags,
		Properties: &armdesktopvirtualization.HostPoolProperties{
			FriendlyName: to.Ptr("Host Pool for AVD stack '" + suffix + "'"),
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
