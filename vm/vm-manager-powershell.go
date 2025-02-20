package vm

import (
	"context"
	_ "embed"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/appliedres/cloudy-azure/storage"
	"github.com/appliedres/cloudy/logging"
)

// BuildVirtualMachineSetupScript dynamically constructs the PowerShell script
func (vmm *AzureVirtualMachineManager) buildVirtualMachineSetupScript(ctx context.Context, config PowershellConfig, hostPoolRegistrationToken *string) (*string, error) {
	log := logging.GetLogger(ctx)

	// Validate required fields dynamically
	if err := validateConfig(config, hostPoolRegistrationToken); err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "Validating config used in powershell builder")
	}

	containerUrl, err := vmm.generateSASToken(ctx, config.BinaryStorage.BlobStorageAccount, config.BinaryStorage.BlobContainer)
	if err != nil {
		return nil, logging.LogAndWrapErr(ctx, log, err, "Generating SAS token for use in powershell script")
	}

	var scriptBuilder strings.Builder

	// Start script
	scriptBuilder.WriteString(GenerateScriptStart() + "\n")

	// Active Directory Join section
	if config.ADJoin != nil {
		scriptBuilder.WriteString(GenerateJoinDomainScript(config.ADJoin) + "\n")
	}

	// AVD Installation section
	if config.AVDInstall != nil {
		scriptBuilder.WriteString(GenerateInstallAvdScript(config.AVDInstall, containerUrl, *hostPoolRegistrationToken) + "\n")
	}

	// Salt Minion Installation section
	if config.SaltMinionInstallConfig != nil {
		scriptBuilder.WriteString(GenerateInstallSaltMinionScript(config.SaltMinionInstallConfig, containerUrl) + "\n")
	}

	// Restart system
	scriptBuilder.WriteString(GenerateRestartScript() + "\n")

	// End script
	scriptBuilder.WriteString(GenerateScriptEnd() + "\n")

	script := scriptBuilder.String()
	return &script, nil
}

// validateConfig dynamically checks required fields for non-nil nested structs
func validateConfig(config PowershellConfig, hostPoolToken *string) error {
	configValue := reflect.ValueOf(config)
	configType := reflect.TypeOf(config)

	for i := 0; i < configValue.NumField(); i++ {
		fieldValue := configValue.Field(i)
		fieldType := configType.Field(i)

		// Check if the field is a pointer to a struct (optional feature block)
		if fieldValue.Kind() == reflect.Ptr && !fieldValue.IsNil() {
			// Validate nested fields
			nestedStruct := fieldValue.Elem()
			nestedStructType := fieldType.Type.Elem()

			for j := 0; j < nestedStruct.NumField(); j++ {
				nestedField := nestedStruct.Field(j)
				nestedFieldType := nestedStructType.Field(j)

				// Skip validation for fields that are nil (they are optional)
				if nestedField.Kind() == reflect.Ptr && nestedField.IsNil() {
					continue
				}

				// Ensure required string fields are not empty
				if nestedField.Kind() == reflect.String && nestedField.String() == "" {
					return fmt.Errorf("%s is required when %s is set", nestedFieldType.Name, fieldType.Name)
				}
			}
		}
	}

	// Validate hostPoolToken only if AVDInstall is enabled
	if config.AVDInstall != nil && hostPoolToken == nil {
		return fmt.Errorf("hostPoolToken is required when AVDInstallConfig is set")
	}

	// Validate BinaryStorage if AVDInstall or SaltMinionInstallConfig are enabled
	if (config.AVDInstall != nil || config.SaltMinionInstallConfig != nil) && config.BinaryStorage == nil {
		if config.BinaryStorage.BlobStorageAccount == "" || config.BinaryStorage.BlobContainer == "" {
			return fmt.Errorf("BlobStorageAccount and BlobContainer are required when AVDInstallConfig or SaltMinionInstallConfig is set")
		}
	}

	return nil
}

//go:embed vm-setup-powershell/0_scriptStart.ps1
var scriptStart string

func GenerateScriptStart() string {
	return scriptStart
}

//go:embed vm-setup-powershell/1_joinDomain.ps1
var joinDomainTemplate string

func GenerateJoinDomainScript(adConfig *ADJoinConfig) string {
	script := joinDomainTemplate

	ouPath := ""  // powershell will handle the empty string appropriately
	if adConfig.OrganizationalUnitPath != nil {
		ouPath = *adConfig.OrganizationalUnitPath
	}

	replacements := map[string]string{
		"$DOMAIN_NAME":              adConfig.DomainName,
		"$DOMAIN_USERNAME":          adConfig.DomainUsername,
		"$DOMAIN_PASSWORD":          adConfig.DomainPassword,
		"$ORGANIZATIONAL_UNIT_PATH": ouPath,
	}

	for key, value := range replacements {
		script = strings.ReplaceAll(script, key, value)
	}

	return script
}

//go:embed vm-setup-powershell/2_installAVD.ps1
var installAvdTemplate string

func GenerateInstallAvdScript(avdConfig *AVDInstallConfig, containerUrl, hostPoolToken string) string {
	script := installAvdTemplate

	replacements := map[string]string{
		"$AZURE_CONTAINER_URI":               containerUrl,
		"$AVD_AGENT_INSTALLER_FILENAME":      avdConfig.AVDAgentInstallerFilename,
		"$AVD_BOOTLOADER_INSTALLER_FILENAME": avdConfig.AVDBootloaderInstallerFilename,
		"$REGISTRATION_TOKEN":                hostPoolToken,
	}

	for key, value := range replacements {
		script = strings.ReplaceAll(script, key, value)
	}

	return script
}

//go:embed vm-setup-powershell/3_installSaltMinion.ps1
var installSaltMinionTemplate string

func GenerateInstallSaltMinionScript(saltConfig *SaltMinionInstallConfig, containerUrl string) string {
	script := installSaltMinionTemplate

	replacements := map[string]string{
		"$AZURE_CONTAINER_URI":            containerUrl,
		"$SALT_MINION_INSTALLER_FILENAME": saltConfig.SaltMinionInstallerFilename,
		"$SALT_MASTER":                    saltConfig.SaltMaster,
	}

	for key, value := range replacements {
		script = strings.ReplaceAll(script, key, value)
	}

	return script
}

//go:embed vm-setup-powershell/4_restart.ps1
var restartScriptTemplate string

const restartDelay = "5" // 5 seconds default delay, to allow script log to close
func GenerateRestartScript() string {
	script := strings.ReplaceAll(restartScriptTemplate, "$RESTART_DELAY", restartDelay)
	return script
}

//go:embed vm-setup-powershell/5_scriptEnd.ps1
var scriptEnd string

func GenerateScriptEnd() string {
	return scriptEnd
}

func (vmm *AzureVirtualMachineManager) generateSASToken(ctx context.Context, storageAccount, container string) (string, error) {
	log := logging.GetLogger(ctx)

	perms := sas.ContainerPermissions{Read: true, List: true}
	validFor := 1 * time.Hour

	sasURL, err := storage.GenerateUserDelegationSAS(ctx, vmm.credentials, storageAccount, container, validFor, perms)
	if err != nil {
		return "", fmt.Errorf("failed to generate SAS token: %w", err)
	}
	log.DebugContext(ctx, "Generated SAS token",
		"storageAccount", storageAccount,
		"container", container,
		"validFor", fmt.Sprintf("%d days %d hours %d minutes", int(validFor.Hours()/24), int(validFor.Hours())%24, int(validFor.Minutes())%60),
		"permissions", perms)

	return sasURL, nil
}
