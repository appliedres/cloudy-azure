package powershell

import (
	_ "embed"
	"fmt"
	"reflect"
	"strings"
)

// PowershellConfig defines the overall configuration with nested structs
// Marking a section nil / not defined will remove it from the generated script 
type PowershellConfig struct {
	ADJoin       		*ADJoinConfig
	AVDInstall   		*AVDInstallConfig
	SaltInstall  		*SaltInstallConfig
	RestartVirtualMachine bool
}

// ADJoinConfig defines the settings required for Active Directory Join
type ADJoinConfig struct {
	DomainName             string
	DomainUsername         string
	DomainPassword         string
	OrganizationalUnitPath string
}

// AVDInstallConfig defines the settings required for AVD installation
type AVDInstallConfig struct {
	AzureContainerUri              string
	AVDAgentInstallerFilename      string
	AVDBootloaderInstallerFilename string
	RegistrationToken              string
}

// SaltInstallConfig defines the settings required for Salt Minion installation
type SaltInstallConfig struct {
	AzureContainerUri	string
	SaltMaster          string
}

// BuildVirtualMachineSetupScript dynamically constructs the PowerShell script
func BuildVirtualMachineSetupScript(config PowershellConfig) (string, error) {
	// Validate required fields dynamically
	if err := validateConfig(config); err != nil {
		return "", err
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
		scriptBuilder.WriteString(GenerateInstallAvdScript(config.AVDInstall) + "\n")
	}

	// Salt Minion Installation section
	if config.SaltInstall != nil {
		scriptBuilder.WriteString(GenerateInstallSaltMinionScript(config.SaltInstall) + "\n")
	}

	// Restart system
	scriptBuilder.WriteString(GenerateRestartScript() + "\n")

	// End script
	scriptBuilder.WriteString(GenerateScriptEnd() + "\n")

	return scriptBuilder.String(), nil
}

// validateConfig dynamically checks required fields for non-nil nested structs
func validateConfig(config PowershellConfig) error {
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

				if nestedField.Kind() == reflect.String && nestedField.String() == "" {
					return fmt.Errorf("%s is required when %s is set", nestedFieldType.Name, fieldType.Name)
				}
			}
		}
	}

	return nil
}

//go:embed 0_scriptStart.ps1
var scriptStart string
func GenerateScriptStart() string {
    return scriptStart
}

//go:embed 1_joinDomain.ps1
var joinDomainTemplate string
func GenerateJoinDomainScript(adConfig *ADJoinConfig) string {
	script := joinDomainTemplate

	replacements := map[string]string{
		"$DOMAIN_NAME":              adConfig.DomainName,
		"$DOMAIN_USERNAME":          adConfig.DomainUsername,
		"$DOMAIN_PASSWORD":          adConfig.DomainPassword,
		"$ORGANIZATIONAL_UNIT_PATH": adConfig.OrganizationalUnitPath,
	}

	for key, value := range replacements {
		script = strings.ReplaceAll(script, key, value)
	}

	return script
}

//go:embed 2_installAVD.ps1
var installAvdTemplate string
func GenerateInstallAvdScript(avdConfig *AVDInstallConfig) string {
	script := installAvdTemplate

	replacements := map[string]string{
		"$AZURE_CONTAINER_URI":               avdConfig.AzureContainerUri,
		"$AVD_AGENT_INSTALLER_FILENAME":      avdConfig.AVDAgentInstallerFilename,
		"$AVD_BOOTLOADER_INSTALLER_FILENAME": avdConfig.AVDBootloaderInstallerFilename,
		"$REGISTRATION_TOKEN":                avdConfig.RegistrationToken,
	}

	for key, value := range replacements {
		script = strings.ReplaceAll(script, key, value)
	}

	return script
}

//go:embed 3_installSaltMinion.ps1
var installSaltMinionTemplate string
func GenerateInstallSaltMinionScript(saltConfig *SaltInstallConfig) string {
	script := installSaltMinionTemplate

	replacements := map[string]string{
		"$AZURE_CONTAINER_URI":            saltConfig.AzureContainerUri,
		"$SALT_MASTER":                    saltConfig.SaltMaster,
	}

	for key, value := range replacements {
		script = strings.ReplaceAll(script, key, value)
	}

	return script
}

//go:embed 4_restart.ps1
var restartScriptTemplate string
const restartDelay = "5" // 5 seconds default delay, to allow script log to close
func GenerateRestartScript() string {
	script := strings.ReplaceAll(restartScriptTemplate, "$RESTART_DELAY", restartDelay)
	return script
}

//go:embed 5_scriptEnd.ps1
var scriptEnd string
func GenerateScriptEnd() string {
	return scriptEnd
}