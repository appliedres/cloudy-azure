package avd

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/appliedres/cloudy"
	"github.com/google/uuid"
)

func (avd *AzureVirtualDesktopManager) extractSuffixFromHostPoolName(hostPoolName string) (string, error) {
	if strings.HasPrefix(hostPoolName, avd.config.HostPoolNamePrefix) {
		return strings.TrimPrefix(hostPoolName, avd.config.HostPoolNamePrefix), nil
	}
	return "", fmt.Errorf("host pool name %s does not have the expected prefix %s", hostPoolName, avd.config.HostPoolNamePrefix)
}

func GenerateNextName(suffixes []string, maxSequences int) (string, error) {
	if len(suffixes) == 0 {
		newName := phoneticAlphabet[0]
		return newName, nil
	}

	var highestSuffix string
	for _, suffix := range suffixes {
		if suffix != "" {
			suffix = strings.ToUpper(suffix)
			if suffix > highestSuffix {
				highestSuffix = suffix
			}
		}
	}

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

func (avd *AzureVirtualDesktopManager) AssignRoleToUser(ctx context.Context, rgName string, roleid string, upn string) error {
	scope := "/subscriptions/" + avd.credentials.SubscriptionID + "/resourceGroups/" + rgName
	roledefid := "/subscriptions/" + avd.credentials.SubscriptionID + "/providers/Microsoft.Authorization/roleDefinitions/" + roleid
	uuidWithHyphen := uuid.New().String()

	res, err := avd.roleAssignmentsClient.Create(ctx, scope, uuidWithHyphen,
		armauthorization.RoleAssignmentCreateParameters{
			Properties: &armauthorization.RoleAssignmentProperties{
				RoleDefinitionID: to.Ptr(roledefid),
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
func generateWindowsClientURI(workspaceID, resourceID, upn, env, version string, useMultiMon bool) string {
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
