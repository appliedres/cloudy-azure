package avd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy"
	"github.com/appliedres/cloudy/logging"
)

// WaitForSessionHost waits for a VM to appear as a session host in a specified host pool and ensures its status is 'Available'.
func (avd *AzureVirtualDesktopManager) WaitForSessionHost(ctx context.Context, rgName, hpName, vmID string, timeout time.Duration) (*armdesktopvirtualization.SessionHost, error) {
	// Set up a timer for the timeout
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Second) // TODO: switch to exponential backoff
	defer ticker.Stop()

	for {
		// TODO: switch to multiple waits - 1 for session host existing, another for when it's ready.

		// Check if the VM is registered as a session host
		sessionHost, err := avd.FindSessionHostByVMNameInHostPool(ctx, rgName, hpName, vmID)
		if err != nil {
			return nil, fmt.Errorf("error finding session host: %w", err)
		}

		// If the session host is found, check its status
		if sessionHost != nil {
			if sessionHost.Properties != nil && sessionHost.Properties.Status != nil {
				if *sessionHost.Properties.Status == armdesktopvirtualization.StatusAvailable {
					// Session host is found and its status is 'Available'
					return sessionHost, nil
				}
			}
		}

		// Check if we've exceeded the timeout
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for session host to become available")
		}

		// Wait for the next polling interval
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			// Continue polling
		}
	}
}

// Searches for a session host with a name that contains the VMs ID
func (avd *AzureVirtualDesktopManager) FindSessionHostByVMNameInHostPool(ctx context.Context, rgName string, hostPoolName string, vmID string) (*armdesktopvirtualization.SessionHost, error) {
	log := logging.GetLogger(ctx)

	log.DebugContext(ctx, "Searching for session host in host pool", "Host Pool Name", hostPoolName)

	allSessionHosts, err := avd.listSessionHosts(ctx, rgName, hostPoolName)
	if err != nil {
		return nil, fmt.Errorf("failed to list session hosts: %w", err)
	}

	for _, sessionHost := range allSessionHosts {
		if sessionHost.Properties != nil && sessionHost.Properties.ResourceID != nil {
			if strings.Contains(*sessionHost.Properties.ResourceID, vmID) {
				return sessionHost, nil
			}
		}
	}

	return nil, nil
}

func (avd *AzureVirtualDesktopManager) listSessionHosts(ctx context.Context, rgName string, hostPoolName string) ([]*armdesktopvirtualization.SessionHost, error) {
	pager := avd.sessionHostsClient.NewListPager(rgName, hostPoolName, nil)
	var all []*armdesktopvirtualization.SessionHost
	for {
		if !pager.More() {
			break
		}
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Value...)
	}
	return all, nil
}

// only used if there is a pool of available VMs to assign to users
func (avd *AzureVirtualDesktopManager) getAvailableSessionHost(ctx context.Context, rgName string, hpname string) (*string, error) {
	sessions, err := avd.listSessionHosts(ctx, rgName, hpname)
	if err != nil {
		return nil, err
	}

	for _, session := range sessions {
		assigned := session.Properties.AssignedUser
		status := session.Properties.Status
		if assigned == nil && *status == "Available" {
			temp := *session.Name
			lastInd := strings.LastIndex(temp, "/")
			if lastInd == -1 {
				return session.Name, nil
			}
			sessionName := temp[lastInd+1:]
			return &sessionName, nil
		}
	}
	return nil, cloudy.Error(ctx, "GetAvailableSessionHost failure (no available session host): %+v", err)
}

// Assigns a User to a session host
func (avd *AzureVirtualDesktopManager) AssignSessionHost(ctx context.Context, rgName string, hpName string, shName string, userobjectid string) error {
	res, err := avd.sessionHostsClient.Update(ctx, rgName, hpName, shName,
		&armdesktopvirtualization.SessionHostsClientUpdateOptions{
			SessionHost: &armdesktopvirtualization.SessionHostPatch{
				Properties: &armdesktopvirtualization.SessionHostPatchProperties{
					AllowNewSession: to.Ptr(true),
					AssignedUser:    to.Ptr(userobjectid),
				},
			},
		})

	if err != nil {
		return cloudy.Error(ctx, "AssignSessionHost failure: %+v", err)
	}
	_ = res

	return nil
}

func (avd *AzureVirtualDesktopManager) DeleteSessionHost(ctx context.Context, rgName string, hpname string, sessionhost string) error {
	// removes session host from host pool, does not delete VM

	res, err := avd.sessionHostsClient.Delete(ctx, rgName, hpname, sessionhost, nil)
	if err != nil {
		return cloudy.Error(ctx, "AssignSessionHost failure: %+v", err)
	}
	_ = res

	return nil
}
