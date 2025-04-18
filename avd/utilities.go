package avd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy"
	"github.com/google/uuid"
)

func (avd *AzureVirtualDesktopManager) extractSuffixFromHostPoolName(hostPoolName string) (string, error) {
	if strings.HasPrefix(hostPoolName, avd.Config.PersonalHostPoolNamePrefix) {
		return strings.TrimPrefix(hostPoolName, avd.Config.PersonalHostPoolNamePrefix), nil
	}
	return "", fmt.Errorf("host pool name %s does not have the expected prefix %s", hostPoolName, avd.Config.PersonalHostPoolNamePrefix)
}

func GenerateNextName(highestSuffix string, maxSequences int) (string, error) {
	if maxSequences < 1 {
		return "", fmt.Errorf("max sequences must be greater than 0")
	}
	if highestSuffix == "" {
		return phoneticAlphabet[0], nil
	}

	highestSuffix = strings.ToUpper(highestSuffix)
	nextSuffix, err := getNextPhoneticWord(highestSuffix, maxSequences)
	if err != nil {
		return "", err
	}

	return nextSuffix, nil
}

var phoneticAlphabet = []string{
	"ALPHA", "BRAVO", "CHARLIE", "DELTA", "ECHO", "FOXTROT", "GOLF", "HOTEL",
	"INDIA", "JULIET", "KILO", "LIMA", "MIKE", "NOVEMBER", "OSCAR", "PAPA", "QUEBEC",
	"ROMEO", "SIERRA", "TANGO", "UNIFORM", "VICTOR", "WHISKEY", "XRAY", "YANKEE", "ZULU",
}

// generateNextWord generates the next word in the phonetic sequence given the current word and max sequences.
func getNextPhoneticWord(current string, maxSequences int) (string, error) {
	parts := strings.Split(current, "-")
	if len(parts) > maxSequences {
		return "", fmt.Errorf("Current word exceeds max sequences param")
	}

	lastWord := parts[len(parts)-1]
	index := indexOf(lastWord, phoneticAlphabet)
	if index == -1 {
		return "", fmt.Errorf("Invalid current word")
	}

	if index < len(phoneticAlphabet)-1 {
		parts[len(parts)-1] = phoneticAlphabet[index+1]
	} else {
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] != phoneticAlphabet[len(phoneticAlphabet)-1] {
				parts[i] = phoneticAlphabet[indexOf(parts[i], phoneticAlphabet)+1]
				break
			} else {
				parts[i] = phoneticAlphabet[0]
				if i == 0 {
					if len(parts) < maxSequences {
						parts = append([]string{phoneticAlphabet[0]}, parts...)
					} else {
						return "", fmt.Errorf("Max sequences exceeded")
					}
				}
			}
		}
	}

	output := strings.Join(parts, "-")
	return output, nil
}

// indexOf returns the index of a word in the phonetic alphabet.
func indexOf(word string, list []string) int {
	for i, w := range list {
		if w == word {
			return i
		}
	}
	return -1
}

func (avd *AzureVirtualDesktopManager) AssignRoleToUser(ctx context.Context, roleID string, upn string) error {
	scope := "/subscriptions/" + avd.Credentials.SubscriptionID + "/resourceGroups/" + avd.Credentials.ResourceGroup
	roleDefID := "/subscriptions/" + avd.Credentials.SubscriptionID + "/providers/Microsoft.Authorization/roleDefinitions/" + roleID
	uuidWithHyphen := uuid.New().String()

	res, err := avd.roleAssignmentsClient.Create(ctx, scope, uuidWithHyphen,
		armauthorization.RoleAssignmentCreateParameters{
			Properties: &armauthorization.RoleAssignmentProperties{
				RoleDefinitionID: to.Ptr(roleDefID),
				PrincipalID:      to.Ptr(upn),
			},
		}, nil)
	if err != nil && strings.Split(err.Error(), "ERROR CODE: RoleAssignmentExists") == nil {
		return cloudy.Error(ctx, "AssignRolesToUser failure: %+v", err)
	}
	_ = res
	return nil
}

// GenerateWindowsClientURI generates a URI for connecting to an AVD session with the Windows client.
// TODO: pass in workspace obj and app / desktop obj (need two different methods)
func (avd *AzureVirtualDesktopManager) GenerateWindowsClientURI(workspaceID, resourceID, upn, env, version string, useMultiMon bool) string {
	// https://learn.microsoft.com/en-us/azure/virtual-desktop/uri-scheme
	base := "ms-avd:connect"

	return fmt.Sprintf(
		"%s?workspaceid=%s&resourceid=%s&username=%s&env=%s&version=%s&usemultimon=%t",
		base,
		workspaceID,
		resourceID,
		upn,
		env,
		version,
		useMultiMon,
	)
}

// map for O(1) look‑ups
var phoneticIndex = func() map[string]int {
	m := make(map[string]int, len(phoneticAlphabet))
	for i, w := range phoneticAlphabet {
		m[w] = i
	}
	return m
}()

// sortHostPoolsByPhoneticSuffix sorts the slice in place so that the pools
// are in phonetic order of the suffix:
//
//	ALPHA … ZULU           ← all single‑word names first
//	ALPHA‑ALPHA … ZULU‑ZULU
func (avd *AzureVirtualDesktopManager) sortHostPoolsByPhoneticSuffix(
	hostPools []*armdesktopvirtualization.HostPool,
) {
	sort.SliceStable(hostPools, func(i, j int) bool {
		si, errI := avd.extractSuffixFromHostPoolName(*hostPools[i].Name)
		sj, errJ := avd.extractSuffixFromHostPoolName(*hostPools[j].Name)

		// If either suffix is invalid, keep original order deterministically
		if errI != nil && errJ != nil {
			return i < j
		}
		if errI != nil {
			return false
		}
		if errJ != nil {
			return true
		}
		return phoneticLess(si, sj)
	})
}

// phoneticLess returns true if a < b according to the required ordering.
//
//   - All single‑word suffixes come before ANY multi‑word suffix.
//   - Within each group, comparison is lexicographic according to the phonetic
//     alphabet for every segment (unknown words sort after known ones).
func phoneticLess(a, b string) bool {
	ta := strings.Split(strings.ToUpper(strings.TrimSpace(a)), "-")
	tb := strings.Split(strings.ToUpper(strings.TrimSpace(b)), "-")

	// Single‑segment names always sort before multi‑segment names
	if len(ta) == 1 && len(tb) > 1 {
		return true
	}
	if len(ta) > 1 && len(tb) == 1 {
		return false
	}

	// Otherwise compare segment‑by‑segment
	for i := 0; i < len(ta) && i < len(tb); i++ {
		ia := idxOrMax(ta[i])
		ib := idxOrMax(tb[i])
		if ia != ib {
			return ia < ib
		}
	}

	// All shared segments equal → shorter one first
	return len(ta) < len(tb)
}

// idxOrMax returns the index of w in the phonetic alphabet, or a large number
// so unknown words always sort after known ones.
func idxOrMax(w string) int {
	if idx, ok := phoneticIndex[w]; ok {
		return idx
	}
	return len(phoneticAlphabet) + 1
}
