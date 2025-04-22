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
func (vdo *VirtualDesktopOrchestrator) purgeStaleHost(ctx context.Context, hostName string) {
    log := logging.GetLogger(ctx)
	pool := vdo.avdManager.Config.PooledHostPoolNamePrefix + vdo.avdManager.Name
    log.DebugContext(ctx, "purgeStaleHost start", "pool", pool, "rawHost", hostName)

    // Normalize host name
    if strings.Contains(hostName, "/") {
        original := hostName
        hostName = hostName[strings.LastIndex(hostName, "/")+1:]
        log.DebugContext(ctx, "Normalized host name", "from", original, "to", hostName)
    }

    // remove session host from AVD
    log.DebugContext(ctx, "Deleting session host from AVD", "pool", pool, "host", hostName)
    if err := vdo.avdManager.DeleteSessionHost(ctx, pool, hostName); err != nil {
        log.WarnContext(ctx, "Failed to delete stale session host object", "pool", pool, "host", hostName, "err", err)
    } else {
        log.DebugContext(ctx, "Deleted stale session host from AVD", "host", hostName)
    }

    // best-effort delete session host VM
    log.DebugContext(ctx, "Deleting associated session host VM", "host", hostName)
    if err := vdo.vmManager.DeleteVirtualMachine(ctx, hostName); err != nil {
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
    log.DebugContext(ctx, "Fetched application groups", "count", len(appGroups))

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
    log.DebugContext(ctx, "Cached all VMs", "count", len(vmMap))

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
            innerLog.DebugContext(ctx, "Validating VM & assignments", "appGroup", agName, "vmID", vmID)

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
    //     if vm.Template.OperatingSystem == "Linux" && vm.CloudState != nil && *vm.CloudState == models.VirtualMachineCloudStateRunning {
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
        staleHosts    []string
    )

    // 3) Categorize hosts by status
    for _, h := range hosts {
        if h.Name == nil || h.Properties == nil || h.Properties.Status == nil {
            if h.Name != nil {
                log.DebugContext(ctx, "Found stale host entry", "host", *h.Name)
                staleHosts = append(staleHosts, *h.Name)
            }
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
            staleHosts = append(staleHosts, *h.Name)
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
        name := *host.Name

        log.DebugContext(ctx, "Starting shutdown session host VM", "host", name)
        if err := vdo.vmManager.StartVirtualMachine(ctx, name); err != nil {
            log.WarnContext(ctx, "StartVirtualMachine failed", "host", name, "err", err)
            // you might choose to append it back: shutdownHosts = append(shutdownHosts, host)
            continue
        }

        if _, err := vdo.avdManager.WaitForSessionHost(ctx, pool, name, 5*time.Minute); err != nil {
            log.WarnContext(ctx, "SessionHost did not become available after start", "host", name, "err", err)
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
    if len(upHosts) >= needHosts {
        log.DebugContext(ctx, "Deleting unused shutdown hosts", "count", len(shutdownHosts))
        for _, host := range shutdownHosts {
            log.DebugContext(ctx, "Deleting unused shutdown host", "host", *host.Name)
            go vdo.purgeStaleHost(ctx, *host.Name)
        }
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
