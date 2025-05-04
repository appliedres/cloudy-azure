package vdo

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	logging "github.com/appliedres/cloudy/logging"
	"github.com/appliedres/cloudy/models"
	"golang.org/x/sync/errgroup"
)

var (
    reservations sync.Map // map[string]*sync.Map (vmID→struct{})
    capacityLock sync.Mutex // single lock for this VDO for all Pooled Host Pool operations
)

type scaleCfg struct {
    MaxSessionsPerHost int32
    MinHosts, MaxHosts int
    DeleteOnScaleDown  bool
}

// TODO: move to vdo config
var tempConfig = scaleCfg{
    MaxSessionsPerHost: 4,
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
    log.DebugContext(ctx, "Attempting to delete associated session host VM if it exists", "host", hostName)
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
    if err != nil {
        log.WarnContext(ctx, "GetAllVirtualMachines failed", "err", err)
    }
    allUserVMs := *allUserVMsPtr
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

// ‼ single global lock for this orchestrator’s Pooled Host Pool operations
var PooledPoolLock sync.Mutex

// ensureCapacity scales session hosts to match the number of VM reservations
func (vdo *VirtualDesktopOrchestrator) ensureCapacity(ctx context.Context) error {
    log := logging.GetLogger(ctx)
    log.DebugContext(ctx, "ensureCapacity triggered")

    pool := vdo.avdManager.Config.PooledHostPoolNamePrefix + vdo.avdManager.Name

    // Prevent concurrent rebuilds
    start := time.Now()
    if !capacityLock.TryLock() { // Go 1.18+.  Returns false if someone else holds it.
        log.InfoContext(ctx, "ensureCapacity already running – waiting for the previous run to finish")
        capacityLock.Lock() // block until the other run unlocks
        log.DebugContext(ctx, "ensureCapacity POOLED ensureCapacity lock acquired after waiting", "waited", time.Since(start).String())
    }
    defer capacityLock.Unlock()

    log.DebugContext(ctx, "ensureCapacity start", "pool", pool)

    // 1) Rebuild reservation state
    if err := vdo.RebuildReservationsFromAppGroups(ctx); err != nil {
        log.ErrorContext(ctx, "RebuildReservationsFromAppGroups failed", "err", err)
        return err
    }
    log.DebugContext(ctx, "ensureCapacity RebuildReservationsFromAppGroups complete")

    // 2) List all session hosts
    hosts, err := vdo.avdManager.ListSessionHosts(ctx, pool)
    if err != nil {
        log.ErrorContext(ctx, "ListSessionHosts failed", "err", err)
        return err
    }
    log.DebugContext(ctx, "ensureCapacity Listed session hosts", "count", len(hosts))

    var (
        upHosts       []*armdesktopvirtualization.SessionHost
        shutdownHosts []*armdesktopvirtualization.SessionHost
        hostsToDelete    []*armdesktopvirtualization.SessionHost
    )

    // 3) Categorize hosts by status
    for _, h := range hosts {
        if h.Properties == nil || h.Properties.Status == nil {
            log.DebugContext(ctx, "ensureCapacity Found session host with empty properties or status. Marking it for deletion", "host", *h.Name)
            hostsToDelete = append(hostsToDelete, h)
            continue
        }
        status := *h.Properties.Status
        log.DebugContext(ctx, "ensureCapacity Host status check", "host", *h.Name, "status", status)
        switch status {
        case armdesktopvirtualization.StatusAvailable:
            log.DebugContext(ctx, "ensureCapacity Host is available. Marking it as 'up'", "host", *h.Name)
            upHosts = append(upHosts, h)
        case armdesktopvirtualization.StatusShutdown:
            // sometimes session hosts report shutdown even if the VM does not exist. We'll verify that it exists.
            _, _, vmName, err := vdo.avdManager.ParseSessionHostName(ctx, h)
            if err != nil {
                log.WarnContext(ctx, "ensureCapacity Failed to parse session host name. Marking it for deletion", "host name", *h.Name, "err", err)
                hostsToDelete = append(hostsToDelete, h)
                continue
            }
            
            vm, err := vdo.vmManager.GetVirtualMachine(ctx, vmName, true)
            if err != nil || vm == nil {
                log.WarnContext(ctx, "ensureCapacity GetVirtualMachine failed for shutdown host. Marking it for deletion", "host", *h.Name, "vmName", vmName, "err", err)
                hostsToDelete = append(hostsToDelete, h)
                continue
            }

            // session host reports stale and VM exists, mark as shutdown
            log.DebugContext(ctx, "ensureCapacity Session host is shutdown amd VM exists. Marking it as shutdown", "host", *h.Name, "vmName", vmName)
            shutdownHosts = append(shutdownHosts, h)
        default:
            log.DebugContext(ctx, "ensureCapacity Host is not available or shutdown. Marking it for deletion", "host", *h.Name, "status", status)
            hostsToDelete = append(hostsToDelete, h)
        }
    }
    log.DebugContext(ctx, "ensureCapacity Host categorization complete",
        "up hosts", len(upHosts),
        "shutdown hosts", len(shutdownHosts),
        "hosts to delete", len(hostsToDelete),
    )

    // 4) Capacity calculation
    currentReservations := vdo.reservationCount(pool)
	neededReservations := currentReservations + 1
    needHosts := int(math.Ceil(float64(neededReservations) / float64(tempConfig.MaxSessionsPerHost)))
    needHosts = clamp(needHosts, tempConfig.MinHosts, tempConfig.MaxHosts)

    log.DebugContext(ctx, "ensureCapacity Capacity calculation",
        "currentReservations", currentReservations,
		"neededReservations", neededReservations,
		"MaxSessionsPerHost", tempConfig.MaxSessionsPerHost,
        "needHosts", needHosts,
    )

    // 5) Delete hosts marked for deletion
    log.DebugContext(ctx, "ensureCapacity Deleting hosts marked for deletion asynchronously", "count", len(hostsToDelete))
    var wg sync.WaitGroup
    for _, host := range hostsToDelete {
        wg.Add(1)
        go func(h *armdesktopvirtualization.SessionHost) {
            defer wg.Done()
            vdo.purgeStaleHost(ctx, h)
        }(host)
    }
    wg.Wait()
    log.DebugContext(ctx, "ensureCapacity Completed deletion of hosts marked for deletion", "deletedCount", len(hostsToDelete))

    // 6) Start shutdown hosts if needed
    var toStart int
    diff := needHosts - len(upHosts)
    if diff > 0 {
        toStart = min(len(shutdownHosts), diff)
    } else {
        toStart = 0
    }
	log.DebugContext(ctx, "ensureCapacity toStart calculation", "toStart", toStart )
    startedHosts := make([]*armdesktopvirtualization.SessionHost, 0, toStart)

    for i := 0; i < toStart; i++ {
        // pop first shutdown host
        host := shutdownHosts[0]
        shutdownHosts = shutdownHosts[1:]

        _, sessionHostName, vmName, err := vdo.avdManager.ParseSessionHostName(ctx, host)
        if err != nil {
            log.WarnContext(ctx, "ensureCapacity Failed to parse session host name", "host name", *host.Name, "err", err) 
            continue
        }
        
        log.DebugContext(ctx, "ensureCapacity Starting shutdown session host VM", "host", sessionHostName, "vmName", vmName)
        if err := vdo.vmManager.StartVirtualMachine(ctx, vmName); err != nil {
            log.WarnContext(ctx, "ensureCapacity StartVirtualMachine failed", "host", sessionHostName, "vmName", vmName, "err", err)
            // you might choose to append it back: shutdownHosts = append(shutdownHosts, host)
            continue
        }

        if _, err := vdo.avdManager.WaitForSessionHost(ctx, pool, vmName, 5*time.Minute); err != nil {
            log.WarnContext(ctx, "ensureCapacity SessionHost did not become available after start", "host", sessionHostName, "vmName", vmName, "err", err)
            continue
        }

        // only on successful start+ready
        upHosts = append(upHosts, host)
        startedHosts = append(startedHosts, host)
    }

    // 7) Provision new hosts if still under capacity
    if len(upHosts) < needHosts {
        toCreate := needHosts - len(upHosts)
        log.DebugContext(ctx, "ensureCapacity Provisioning new hosts", "toCreate", toCreate)
        for i := 0; i < toCreate; i++ {
            newHostID := len(upHosts) + 1
            log.DebugContext(ctx, "ensureCapacity Creating new session host", "toStartIndex", i+1, "newHostID", newHostID)
            createdHost, err := vdo.CreateSessionHost(ctx, pool, newHostID) 
            if err != nil || createdHost == nil {
                log.WarnContext(ctx, "ensureCapacity CreateSessionHost failed", "index", i+1, "err", err)
            } else {
                log.DebugContext(ctx, "ensureCapacity Created new session host VM. Marking it as up", "host", *createdHost.Name)
                upHosts = append(upHosts, createdHost)
            }
        }    
    }

    // 8) Delete any unused shutdown hosts once capacity is met
    if len(upHosts) >= needHosts && len(shutdownHosts) > 0 {
        log.DebugContext(ctx, "ensureCapacity Deleting unused shutdown hosts", "count", len(shutdownHosts))
        var wg sync.WaitGroup
        for _, host := range shutdownHosts {
            log.DebugContext(ctx, "ensureCapacity Deleting unused shutdown host", "host", *host.Name)
            wg.Add(1)
            go func(h *armdesktopvirtualization.SessionHost) {
                defer wg.Done()
                vdo.purgeStaleHost(ctx, h)
            }(host)
        }
        wg.Wait()
    }
    log.DebugContext(ctx, "ensureCapacity shutdown session host VM sweep complete")

    // FIXME: disabled for now. this may have been deleting session host VMs that were part of different API deployments.
    // We meed tp make sure the SHVMs we do retrieve are associated to this VDO / Pooled Host Pool.

    // // 9) Find any remaining VMs with the "shvm-" prefix that are NOT represented
    // //       by an Azure Virtual Desktop session-host object.
    // //    These are likely orphaned and should be deleted.

    // // 9a) Build a set of VM names that do have valid session host objects.
    // upHostVMIDs := make(map[string]struct{}, len(hosts))
    // for _, h := range upHosts {
    //     if _, _, vmName, err := vdo.avdManager.ParseSessionHostName(ctx, h); err == nil {
    //         upHostVMIDs[vmName] = struct{}{}
    //     }
    // }

    // // // 9b) Retrieve every VM whose name begins with the shvm- prefix.
    // //     (The helper already filters by the prefix; set resourceGroup=nil to search subscription-wide.)
    // hostVMsPtr, err := vdo.vmManager.GetAllSessionHostVirtualMachines(ctx, nil, true)
    // hostVMs := *hostVMsPtr
    // if err != nil {
    //     log.WarnContext(ctx, "ensureCapacity GetAllSessionHostVirtualMachines failed during shvm orphan check", "err", err)
    // } else {
    //     var orphaned []string
    //     for _, vm := range hostVMs {
    //         // Double-check the prefix in case the helper’s filtering ever changes.
    //         if !strings.HasPrefix(vm.ID, "shvm-") {
    //             log.WarnContext(ctx, "ensureCapacity Session Host VM retrieval found VM with bad prefix", "vmid", vm.ID)
    //             continue
    //         }
    //         if _, knownHost := upHostVMIDs[vm.ID]; knownHost {
    //             continue // still an active or shutdown session host
    //         }

    //         // 9c) VM is orphaned – delete it asynchronously.
    //         orphaned = append(orphaned, vm.ID)
    //         log.WarnContext(ctx, "ensureCapacity Deleting orphaned session host VM", "vmid", vm.ID)
    //         go func(vmid string) {
    //             if err := vdo.vmManager.DeleteVirtualMachine(ctx, vmid); err != nil {
    //                 log.WarnContext(ctx, "ensureCapacity Deleting Orphaned Session Host VM failed", "vmid", vmid, "err", err)
    //             } else {
    //                 log.WarnContext(ctx, "ensureCapacity Orphaned session host VM deleted", "vmid", vmid)
    //             }
    //         }(vm.ID)
    //     }
    //     log.DebugContext(ctx, "ensureCapacity Orphaned session host VM sweep complete", "count", len(orphaned))
    // }

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
