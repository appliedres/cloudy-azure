package vdo

import (
	"context"
	_ "embed"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy-azure/storage"
	"github.com/appliedres/cloudy/logging"
)

// BuildVirtualMachineSetupScript dynamically constructs the PowerShell script
func (vdo VirtualDesktopOrchestrator) buildVirtualMachineSetupScript(ctx context.Context, config VirtualDesktopOrchestratorConfig, hostPoolRegistrationToken *string) (*string, error) {
	log := logging.GetLogger(ctx)

	// TODO: validate VDO config
	// if err := validateConfig(config, hostPoolRegistrationToken); err != nil {
	// 	return nil, logging.LogAndWrapErr(ctx, log, err, "Validating config used in powershell builder")
	// }

	var scriptBuilder strings.Builder

	// Start script
	scriptBuilder.WriteString(GenerateScriptStart() + "\n")

	// Active Directory Join section
	if config.AD != nil {
		scriptBuilder.WriteString(GenerateJoinDomainScript(config.AD) + "\n")
	}

	// AVD Installation section
	if config.AVD != nil {
		avdScript, err := GenerateInstallAvdScript(ctx, vdo.vmManager.Credentials, config.BinaryStorage.BlobStorageAccount, config.BinaryStorage.BlobContainer,
			&config.AVD.InstallerConfig, *hostPoolRegistrationToken)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "Generating AVD Install script component")
		}
		scriptBuilder.WriteString(avdScript + "\n")
	}

	// Salt Minion Installation section
	if config.SaltMinionInstall != nil {
		saltScript, err := GenerateInstallSaltMinionScript(ctx, vdo.vmManager.Credentials, config.BinaryStorage.BlobStorageAccount, config.BinaryStorage.BlobContainer, config.SaltMinionInstall)
		if err != nil {
			return nil, logging.LogAndWrapErr(ctx, log, err, "Generating Salt Minion Install script component")
		}
		scriptBuilder.WriteString(saltScript + "\n")
	}

	// Restart system
	scriptBuilder.WriteString(GenerateRestartScript() + "\n")

	// End script
	scriptBuilder.WriteString(GenerateScriptEnd() + "\n")

	script := scriptBuilder.String()
	return &script, nil
}

// validateConfig dynamically checks required fields for non-nil nested structs
func validateConfig(config VirtualDesktopOrchestratorConfig, hostPoolToken *string) error {
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
	if config.AVD != nil && hostPoolToken == nil {
		// TODO: this causes an error if VMM.AVDManager is nil, which is not the same as AVDInstall being nil
		return fmt.Errorf("hostPoolToken is required when AVDInstallConfig is set")
	}

	// Validate BinaryStorage if AVDInstall or SaltMinionInstallConfig are enabled
	if (config.AVD != nil || config.SaltMinionInstall != nil) && config.BinaryStorage != nil {
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

	ouPath := "" // powershell will handle the empty string appropriately
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

func GenerateInstallAvdScript(ctx context.Context, creds *cloudyazure.AzureCredentials, storageAccountName, containerName string, avdConfig *AVDInstallConfig, hostPoolToken string) (string, error) {
	validFor := 1 * time.Hour

	avdAgentURL, err := storage.GenerateBlobSAS(ctx, creds, storageAccountName, containerName, avdConfig.AVDAgentInstallerFilename, validFor, sas.BlobPermissions{Read: true})
	if err != nil {
		return "", fmt.Errorf("failed to generate SAS for AVD Agent: %w", err)
	}

	avdBootloaderURL, err := storage.GenerateBlobSAS(ctx, creds, storageAccountName, containerName, avdConfig.AVDBootloaderInstallerFilename, validFor, sas.BlobPermissions{Read: true})
	if err != nil {
		return "", fmt.Errorf("failed to generate SAS for AVD Bootloader: %w", err)
	}

	script := installAvdTemplate

	replacements := map[string]string{
		"$AZURE_AVD_AGENT_URL":      avdAgentURL,
		"$AZURE_AVD_BOOTLOADER_URL": avdBootloaderURL,
		"$REGISTRATION_TOKEN":       hostPoolToken,
	}

	for key, value := range replacements {
		script = strings.ReplaceAll(script, key, value)
	}

	return script, nil
}

//go:embed vm-setup-powershell/3_installSaltMinion.ps1
var installSaltMinionTemplate string

func GenerateInstallSaltMinionScript(ctx context.Context, creds *cloudyazure.AzureCredentials, storageAccountName, containerName string, saltConfig *SaltMinionInstallConfig) (string, error) {
	log := logging.GetLogger(ctx)

	validFor := 1 * time.Hour

	saltInstallerURL, err := storage.GenerateBlobSAS(ctx, creds, storageAccountName, containerName, saltConfig.SaltMinionMsiFilename, validFor, sas.BlobPermissions{Read: true})
	if err != nil {
		return "", fmt.Errorf("failed to generate SAS for Salt Minion Installer: %w", err)
	}

	script := installSaltMinionTemplate

	log.DebugContext(ctx, "Generated salt minion install script using Salt Master IP/hostname '%s'", saltConfig.SaltMaster)
	replacements := map[string]string{
		"$AZURE_SALT_MINION_URL": saltInstallerURL,
		"$SALT_MASTER":           saltConfig.SaltMaster,
	}

	for key, value := range replacements {
		script = strings.ReplaceAll(script, key, value)
	}

	log.DebugContext(ctx, fmt.Sprintf("Generated salt minion install script using Salt Master IP/hostname '%s'", saltConfig.SaltMaster))
	return script, nil
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
