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
func (avd *AzureVirtualDesktopManager) WaitForSessionHost(
	ctx context.Context,
	hpName, vmID string,
	timeout time.Duration,
) (*armdesktopvirtualization.SessionHost, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "WaitForSessionHost start",
		"HostPool", hpName,
		"VMID", vmID,
		"Timeout", timeout,
	)

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Second) // TODO: switch to exponential backoff
	defer ticker.Stop()

	for {
		log.DebugContext(ctx, "Polling for session host", "HostPool", hpName, "VMID", vmID)

		sessionHost, err := avd.FindSessionHostByVMNameInHostPool(ctx, hpName, vmID)
		if err != nil {
			log.ErrorContext(ctx, "Error finding session host", "error", err)
			return nil, fmt.Errorf("error finding session host: %w", err)
		}

		if sessionHost == nil {
			log.DebugContext(ctx, "Session host not yet registered", "HostPool", hpName, "VMID", vmID)
		} else if sessionHost.Properties == nil || sessionHost.Properties.Status == nil {
			log.DebugContext(ctx, "Session host found but status is unknown", "SessionHost", *sessionHost.Name)
		} else {
			status := *sessionHost.Properties.Status
			log.DebugContext(ctx, "Session host status", "SessionHost", *sessionHost.Name, "Status", status)
			if status == armdesktopvirtualization.StatusAvailable {
				log.DebugContext(ctx, "Session host is now available", "SessionHost", *sessionHost.Name)
				return sessionHost, nil
			}
		}

		if time.Now().After(deadline) {
			log.WarnContext(ctx, "Timed out waiting for session host to become available",
				"HostPool", hpName,
				"VMID", vmID,
				"Deadline", deadline,
			)
			return nil, fmt.Errorf("timed out waiting for session host to become available")
		}

		select {
		case <-ctx.Done():
			log.WarnContext(ctx, "Context cancelled while waiting for session host", "error", ctx.Err())
			return nil, ctx.Err()
		case <-ticker.C:
			log.DebugContext(ctx, "Retrying WaitForSessionHost loop", "HostPool", hpName, "VMID", vmID)
		}
	}
}

// FindSessionHostByVMNameInHostPool searches for a session host whose ResourceID contains the VM's ID.
func (avd *AzureVirtualDesktopManager) FindSessionHostByVMNameInHostPool(
	ctx context.Context,
	hostPoolName, vmID string,
) (*armdesktopvirtualization.SessionHost, error) {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "FindSessionHostByVMNameInHostPool start",
		"HostPool", hostPoolName,
		"VMID", vmID,
	)

	allSessionHosts, err := avd.ListSessionHosts(ctx, hostPoolName)
	if err != nil {
		log.ErrorContext(ctx, "Failed to list session hosts", "HostPool", hostPoolName, "error", err)
		return nil, fmt.Errorf("failed to list session hosts: %w", err)
	}
	log.DebugContext(ctx, "Listed session hosts count", "HostPool", hostPoolName, "Count", len(allSessionHosts))

	for _, sessionHost := range allSessionHosts {
		rid := ""
		if sessionHost.Properties != nil && sessionHost.Properties.ResourceID != nil {
			rid = *sessionHost.Properties.ResourceID
		}

		if strings.Contains(rid, vmID) {
			log.DebugContext(ctx, "Found matching session host", "HostPool", hostPoolName, "VMID", vmID, "SessionHost", *sessionHost.Name)
			return sessionHost, nil
		}
	}

	log.DebugContext(ctx, "No session host found matching VMID", "HostPool", hostPoolName, "VMID", vmID)
	return nil, nil
}

func (avd *AzureVirtualDesktopManager) ListSessionHosts(ctx context.Context, hostPoolName string) ([]*armdesktopvirtualization.SessionHost, error) {
	pager := avd.sessionHostsClient.NewListPager(avd.Credentials.ResourceGroup, hostPoolName, nil)
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
func (avd *AzureVirtualDesktopManager) getAvailableSessionHost(ctx context.Context, hostPoolName string) (*string, error) {
	sessions, err := avd.ListSessionHosts(ctx, hostPoolName)
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
func (avd *AzureVirtualDesktopManager) AssignSessionHost(ctx context.Context, hpName string, shName string, userobjectid string) error {
	res, err := avd.sessionHostsClient.Update(ctx, avd.Credentials.ResourceGroup, hpName, shName,
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

// removes session host from host pool, does not delete VM
func (avd *AzureVirtualDesktopManager) DeleteSessionHost(ctx context.Context, host *armdesktopvirtualization.SessionHost) error {
	poolName, hostName, _, err := avd.ParseSessionHostName(ctx, host)
	if err != nil {
		return cloudy.Error(ctx, "DeleteSessionHost - couldn't parse host: %+v", err)
	}

	res, err := avd.sessionHostsClient.Delete(ctx, avd.Credentials.ResourceGroup, poolName, hostName, nil)
	if err != nil {
		return cloudy.Error(ctx, "DeleteSessionHost failure: %+v", err)
	}
	_ = res

	return nil
}

// Parses session host name, VM name, and host pool name from a session host object.
// example:
//
//	sessionHost.Name = "E2E-TEST-JDUPRAS-HP-Pooled-_root/shvm-0m9rf333q1.dev.arkloud-pvf.local"
//
// returns:
//
//	hostPoolName     = "E2E-TEST-JDUPRAS-HP-Pooled-_root"
//	sessionHostName  = "shvm-0m9rf333q1.dev.arkloud-pvf.local"
//	vmName           = "shvm-0m9rf333q1"
func (avd *AzureVirtualDesktopManager) ParseSessionHostName(
	ctx context.Context,
	sessionHost *armdesktopvirtualization.SessionHost,
) (hostPoolName, sessionHostName, VMID string, err error) {
	log := logging.GetLogger(ctx)

	// Expect Name like "hostPoolName/shvm-0123456789.domain.local"
	parts := strings.SplitN(*sessionHost.Name, "/", 2)
	if len(parts) != 2 {
		err = fmt.Errorf("could not split sessionHost.Name: %s", *sessionHost.Name)
		return "", "", "", err
	}

	// 1) host pool name is everything before the slash
	hostPoolName = parts[0]

	// 2) session host full DNS name is everything after the slash
	sessionHostName = parts[1]

	// 3) vmName is the hostname without domain suffix
	nameParts := strings.SplitN(sessionHostName, ".", 2)
	VMID = nameParts[0]

	log.DebugContext(ctx, "parsed session host name", "pool", hostPoolName, "host", sessionHostName, "vmid", VMID)
	return hostPoolName, sessionHostName, VMID, nil
}
