package vdo

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	"golang.org/x/sync/errgroup"
)

var (
    reservations sync.Map // map[string]*sync.Map (vmID→struct{})
    capLocks     sync.Map // map[string]*sync.Mutex
)

type scaleCfg struct {
    MaxSessionsPerHost int32
    MinHosts, MaxHosts int
    DeleteOnScaleDown  bool
}

// TODO: move to vdo config
var tempConfig = scaleCfg{
    MaxSessionsPerHost: 2,
    MinHosts:           1,
    MaxHosts:           10,
    DeleteOnScaleDown:  true,
}

// reservationCount returns the number of reserved VMs in pool
func (vdo *VirtualDesktopOrchestrator) reservationCount(pool string) int {
    count := 0
    if m, ok := reservations.Load(pool); ok {
        vmMap := m.(*sync.Map)
        vmMap.Range(func(_, _ interface{}) bool {
            count++
            return true
        })
    }
    return count
}

// reservationVMs returns the list of reserved VMIDs in pool
func (vdo *VirtualDesktopOrchestrator) reservationVMs(pool string) []string {
    var ids []string
    if m, ok := reservations.Load(pool); ok {
        vmMap := m.(*sync.Map)
        vmMap.Range(func(k, _ interface{}) bool {
            ids = append(ids, k.(string))
            return true
        })
    }
    return ids
}

// purgeStaleHost deletes both the AVD session‐host record and its VM
func (vdo *VirtualDesktopOrchestrator) purgeStaleHost(ctx context.Context, host *armdesktopvirtualization.SessionHost) {
    log := logging.GetLogger(ctx)
	// pool := vdo.avdManager.Config.PooledHostPoolNamePrefix + vdo.avdManager.Name

    poolName, hostName, hostVMID, err := vdo.avdManager.ParseSessionHostName(ctx, host)
    if err != nil {
        log.WarnContext(ctx, "Failed to parse session host name", "host name", *host.Name, "err", err)
        return
    }

    log.DebugContext(ctx, "purgeStaleHost start", "pool", poolName, "rawHost", hostName)

    // Normalize host name
    if strings.Contains(hostName, "/") {
        original := hostName
        hostName = hostName[strings.LastIndex(hostName, "/")+1:]
        log.DebugContext(ctx, "Normalized host name", "from", original, "to", hostName)
    }

    // remove session host from AVD
    log.DebugContext(ctx, "Deleting session host from AVD", "pool", poolName, "host", hostName)
    if err := vdo.avdManager.DeleteSessionHost(ctx, host); err != nil {
        log.WarnContext(ctx, "Failed to delete stale session host object", "pool", poolName, "host", hostName, "err", err)
    } else {
        log.DebugContext(ctx, "Deleted stale session host from AVD", "host", hostName)
    }

    // best-effort delete session host VM
    log.DebugContext(ctx, "Deleting associated session host VM", "host", hostName)
    if err := vdo.vmManager.DeleteVirtualMachine(ctx, hostVMID); err != nil {
        log.WarnContext(ctx, "Failed to delete session host VM", "host", hostName, "err", err)
    } else {
        log.DebugContext(ctx, "Deleted session host VM", "host", hostName)
    }

    log.DebugContext(ctx, "purgeStaleHost complete", "host", hostName)
}


// RebuildReservationsFromAppGroups repopulates the reservation set from AVD app groups
func (vdo *VirtualDesktopOrchestrator) RebuildReservationsFromAppGroups(ctx context.Context) error {
    log := logging.GetLogger(ctx)
    pool := vdo.avdManager.Config.PooledHostPoolNamePrefix + vdo.avdManager.Name
    log.InfoContext(ctx, "Rebuilding reservations", "pool", pool)

    // 1) List app groups
    appGroups, err := vdo.avdManager.ListApplicationGroupsForHostPool(ctx, pool)
    if err != nil {
        log.ErrorContext(ctx, "ListApplicationGroupsForHostPool failed", "err", err)
        return err
    }
    log.DebugContext(ctx, "Fetched application groups for host pool", "count", len(appGroups))

    // 2) Fetch all user VMs once
    allUserVMsPtr, err := vdo.vmManager.GetAllUserVirtualMachines(ctx, nil, true)
	allUserVMs := *allUserVMsPtr
    if err != nil {
        log.WarnContext(ctx, "GetAllVirtualMachines failed", "err", err)
    }
    vmMap := make(map[string]*models.VirtualMachine, len(allUserVMs))
    for i := range allUserVMs {
        vm := allUserVMs[i]
        vmMap[vm.ID] = &vm
    }
    log.DebugContext(ctx, "cached all VMs during app group processing", "count", len(vmMap))

    // 3) Validate each app group in parallel
    fresh := &sync.Map{}
    eg := new(errgroup.Group)
    var mu sync.Mutex
    validCount := 0

    for _, ag := range appGroups {
        if ag.Name == nil {
            log.DebugContext(ctx, "Skipping app group with nil name")
            continue
        }
        agName := *ag.Name
        vmID, err := vdo.avdManager.ParseVMIDFromLinuxAVDAppGroupName(agName)
		if err != nil {
			return fmt.Errorf("couldn't parse VM ID from App Group Name: %v", err)
		}
        log.DebugContext(ctx, "Examining app group", "appGroup", agName, "vmID", vmID)

        eg.Go(func() error {
            innerLog := logging.GetLogger(ctx)
            innerLog.DebugContext(ctx, "Validating VM and assignments", "appGroup", agName, "vmID", vmID)

            // VM must exist & be running
            vm, found := vmMap[vmID]
            if !found || vm == nil || vm.CloudState == nil || *vm.CloudState != models.VirtualMachineCloudStateRunning {
                if !found {
                    innerLog.WarnContext(ctx, "VM not found in cache, deleting app group", "appGroup", agName)
                } else {
                    innerLog.WarnContext(ctx, "VM not running, deleting app group", "appGroup", agName, "state", vm.CloudState)
                }
                if delErr := vdo.avdManager.DeleteApplicationGroup(ctx, agName); delErr != nil {
                    innerLog.WarnContext(ctx, "Failed to delete stale app group", "appGroup", agName, "err", delErr)
                }
                return nil
            }

            // Must have at least one assignment
            assigns, err := vdo.avdManager.ListAppGroupAssignments(ctx, agName)
            if err != nil {
                innerLog.WarnContext(ctx, "ListAppGroupAssignments error", "appGroup", agName, "err", err)
                return nil
            }
            if len(assigns) == 0 {
                innerLog.WarnContext(ctx, "No assignments, deleting app group", "appGroup", agName)
                if delErr := vdo.avdManager.DeleteApplicationGroup(ctx, agName); delErr != nil {
                    innerLog.WarnContext(ctx, "Failed to delete unassigned app group", "appGroup", agName, "err", delErr)
                }
                return nil
            }

            // Record as valid
            fresh.Store(vmID, struct{}{})
            mu.Lock()
            validCount++
            mu.Unlock()
            innerLog.DebugContext(ctx, "Recorded reservation", "vmID", vmID)
            return nil
        })
    }

    // 4) Wait for validations to complete
    if err := eg.Wait(); err != nil {
        log.WarnContext(ctx, "Validation errgroup finished with error", "err", err)
    } else {
        log.DebugContext(ctx, "All validation goroutines complete")
    }

    // // 5) Warn for running Linux VMs missing app groups
	// TODO: get all either gets status OR os type, not both at once
    // for id, vm := range vmMap {
    //     if vm.Template.OperatingSystem ==  && vm.CloudState != nil && *vm.CloudState == models.VirtualMachineCloudStateRunning {
    //         if _, found := fresh.Load(id); !found {
    //             log.WarnContext(ctx, "Running Linux VM missing AVD app group", "vmID", id)
    //         }
    //     }
    // }

    // 6) Store rebuilt reservations
    reservations.Store(pool, fresh)
    log.InfoContext(ctx, "Rebuilt reservations complete", "pool", pool, "count", validCount)
    return nil
}

// ensureCapacity scales session hosts to match the number of VM reservations
func (vdo *VirtualDesktopOrchestrator) ensureCapacity(ctx context.Context) error {
    log := logging.GetLogger(ctx)
    pool := vdo.avdManager.Config.PooledHostPoolNamePrefix + vdo.avdManager.Name
    log.DebugContext(ctx, "ensureCapacity start", "pool", pool)

    // Serialize concurrent operations
    lockVal, _ := capLocks.LoadOrStore(pool, &sync.Mutex{})
    mu := lockVal.(*sync.Mutex)
    log.DebugContext(ctx, "Acquiring capacity lock")
    mu.Lock()
    log.DebugContext(ctx, "Capacity lock acquired")

    // 1) Rebuild reservation state
    if err := vdo.RebuildReservationsFromAppGroups(ctx); err != nil {
        log.ErrorContext(ctx, "RebuildReservationsFromAppGroups failed", "err", err)
        mu.Unlock()
        log.DebugContext(ctx, "Capacity lock released after rebuild failure")
        return err
    }
    log.DebugContext(ctx, "RebuildReservationsFromAppGroups complete")

    // 2) List all session hosts
    hosts, err := vdo.avdManager.ListSessionHosts(ctx, pool)
    if err != nil {
        log.ErrorContext(ctx, "ListSessionHosts failed", "err", err)
        mu.Unlock()
        log.DebugContext(ctx, "Capacity lock released after list failure")
        return err
    }
    log.DebugContext(ctx, "Listed session hosts", "count", len(hosts))

    var (
        upHosts       []*armdesktopvirtualization.SessionHost
        shutdownHosts []*armdesktopvirtualization.SessionHost
        staleHosts    []*armdesktopvirtualization.SessionHost
    )

    // 3) Categorize hosts by status
    for _, h := range hosts {
        if h.Properties == nil || h.Properties.Status == nil {
            log.DebugContext(ctx, "Found stale host entry", "host", *h.Name)
            staleHosts = append(staleHosts, h)
            continue
        }
        status := *h.Properties.Status
        log.DebugContext(ctx, "Host status check", "host", *h.Name, "status", status)
        switch status {
        case armdesktopvirtualization.StatusAvailable:
            upHosts = append(upHosts, h)
        case armdesktopvirtualization.StatusShutdown:
            shutdownHosts = append(shutdownHosts, h)
        default:
            staleHosts = append(staleHosts, h)
        }
    }
    log.DebugContext(ctx, "Host categorization complete",
        "available", len(upHosts),
        "shutdown", len(shutdownHosts),
        "stale", len(staleHosts),
    )

    // 4) Capacity calculation
    currentReservations := vdo.reservationCount(pool)
	neededReservations := currentReservations + 1
    needHosts := int(math.Ceil(float64(neededReservations) / float64(tempConfig.MaxSessionsPerHost)))
    needHosts = clamp(needHosts, tempConfig.MinHosts, tempConfig.MaxHosts)

    log.DebugContext(ctx, "Capacity calculation",
        "currentReservations", currentReservations,
		"neededReservations", neededReservations,
		"MaxSessionsPerHost", tempConfig.MaxSessionsPerHost,
        "needHosts", needHosts,
    )

    mu.Unlock()
    log.DebugContext(ctx, "Capacity lock released")

    // 5) Purge stale hosts asynchronously
    log.DebugContext(ctx, "Purging stale hosts asynchronously", "count", len(staleHosts))
    for _, host := range staleHosts {
        go vdo.purgeStaleHost(ctx, host)
    }

    // 6) Start shutdown hosts if needed
    toStart := min(len(shutdownHosts), needHosts-len(upHosts))
	log.DebugContext(ctx, "toStart calculation", "toStart", toStart )
    startedHosts := make([]*armdesktopvirtualization.SessionHost, 0, toStart)

    for i := 0; i < toStart; i++ {
        // pop first shutdown host
        host := shutdownHosts[0]
        shutdownHosts = shutdownHosts[1:]

        _, sessionHostName, vmName, err := vdo.avdManager.ParseSessionHostName(ctx, host)
        if err != nil {
            log.WarnContext(ctx, "Failed to parse session host name", "host name", *host.Name, "err", err) 
            continue
        }
        
        log.DebugContext(ctx, "Starting shutdown session host VM", "host", sessionHostName, "vmName", vmName)
        if err := vdo.vmManager.StartVirtualMachine(ctx, vmName); err != nil {
            log.WarnContext(ctx, "StartVirtualMachine failed", "host", sessionHostName, "vmName", vmName, "err", err)
            // you might choose to append it back: shutdownHosts = append(shutdownHosts, host)
            continue
        }

        if _, err := vdo.avdManager.WaitForSessionHost(ctx, pool, vmName, 5*time.Minute); err != nil {
            log.WarnContext(ctx, "SessionHost did not become available after start", "host", sessionHostName, "vmName", vmName, "err", err)
            continue
        }

        // only on successful start+ready
        upHosts = append(upHosts, host)
        startedHosts = append(startedHosts, host)
    }

    // 7) Provision new hosts if still under capacity
    if len(upHosts) < needHosts {
        toCreate := needHosts - len(upHosts)
        log.DebugContext(ctx, "Provisioning new hosts", "toCreate", toCreate)
        for i := 0; i < toCreate; i++ {
            log.DebugContext(ctx, "Creating new session host", "index", i+1)
            if _, err := vdo.CreateSessionHost(ctx, pool); err != nil {
                log.WarnContext(ctx, "CreateSessionHost failed", "index", i+1, "err", err)
            }
        }
    }

    // 8) Delete any unused shutdown hosts once capacity is met
    if len(upHosts) >= needHosts  && len(shutdownHosts) > 0  {
        log.DebugContext(ctx, "Deleting unused shutdown hosts", "count", len(shutdownHosts))
        for _, host := range shutdownHosts {
            log.DebugContext(ctx, "Deleting unused shutdown host", "host", *host.Name)
            go vdo.purgeStaleHost(ctx, host)
        }
    }
    log.DebugContext(ctx, "shutdown session host VM sweep complete")

    // 9) Find any remaining VMs with the "shvm-" prefix that are NOT represented
    //       by an Azure Virtual Desktop session-host object.
    //    These are likely orphaned and should be deleted.

    // 9a) Build a set of VM names that do have session-host records.
    knownSHVMs := make(map[string]struct{}, len(hosts))
    for _, h := range hosts {
        if _, _, vmName, err := vdo.avdManager.ParseSessionHostName(ctx, h); err == nil {
            knownSHVMs[vmName] = struct{}{}
        }
    }

    // 9b) Retrieve every VM whose name begins with the shvm- prefix.
    //     (The helper already filters by the prefix; set resourceGroup=nil to search subscription-wide.)
    hostVMsPtr, err := vdo.vmManager.GetAllSessionHostVirtualMachines(ctx, nil, true)
    hostVMs := *hostVMsPtr
    if err != nil {
        log.WarnContext(ctx, "GetAllSessionHostVirtualMachines failed during shvm orphan check", "err", err)
    } else {
        var orphaned []string
        for _, vm := range hostVMs {
            // Double-check the prefix in case the helper’s filtering ever changes.
            if !strings.HasPrefix(vm.ID, "shvm-") {
                log.WarnContext(ctx, "Session Host VM retrieval found VM with bad prefix", "vmid", vm.ID)
                continue
            }
            if _, knownHost := knownSHVMs[vm.ID]; knownHost {
                continue // still an active or shutdown session host
            }

            // 9c) VM is orphaned – delete it asynchronously.
            orphaned = append(orphaned, vm.ID)
            log.WarnContext(ctx, "Deleting orphaned session host VM", "vmid", vm.ID)
            go func(vmid string) {
                if err := vdo.vmManager.DeleteVirtualMachine(ctx, vmid); err != nil {
                    log.WarnContext(ctx, "Deleting Orphaned Session Host VM failed", "vmid", vmid, "err", err)
                } else {
                    log.WarnContext(ctx, "Orphaned session host VM deleted", "vmid", vmid)
                }
            }(vm.ID)
        }
        log.DebugContext(ctx, "Orphaned session host VM sweep complete", "count", len(orphaned))
    }

    log.DebugContext(ctx, "ensureCapacity complete",
        "finalUp", len(upHosts),
        "target", needHosts,
    )
    return nil
}

func clamp(x, minVal, maxVal int) int {
    if x < minVal {
        return minVal
    }
    if x > maxVal {
        return maxVal
    }
    return x
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
