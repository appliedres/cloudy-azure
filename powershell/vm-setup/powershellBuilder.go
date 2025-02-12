package powershell

import (
	_ "embed"
	"strings"
)

// ScriptConfig defines the configuration for generating the PowerShell script
type PowershellConfig struct {
	EnableADJoin    bool
	EnableAVDInstall bool
	EnableSaltInstall bool

	// AD Join Parameters
	DomainName              string
	DomainUsername          string
	DomainPassword          string
	OrganizationalUnitPath  string

    // AVD and Salt
    AzureContainerUri         string

	// AVD Install Parameters
	AVDAgentInstallerFilename string
	AVDBootloaderInstallerFilename string
	RegistrationToken         string

	// Salt Minion Install Parameters
	SaltMaster string
}

// GenerateFullScript dynamically constructs the PowerShell script using a configuration struct
func GenerateFullPowershell(config PowershellConfig) string {
	var scriptBuilder strings.Builder

	// Start script
	scriptBuilder.WriteString(GenerateScriptStart() + "\n")

	// Active Directory Join section
	if config.EnableADJoin {
		scriptBuilder.WriteString(GenerateJoinDomainScript(
			config.DomainName,
			config.DomainUsername,
			config.DomainPassword,
			config.OrganizationalUnitPath,
		) + "\n")
	}

	// AVD Installation section
	if config.EnableAVDInstall {
		scriptBuilder.WriteString(GenerateInstallAvdScript(
			config.AzureContainerUri,
			config.AVDAgentInstallerFilename,
			config.AVDBootloaderInstallerFilename,
			config.RegistrationToken,
		) + "\n")
	}

	// Salt Minion Installation section
	if config.EnableSaltInstall {
		scriptBuilder.WriteString(GenerateInstallSaltMinionScript(
			config.AzureContainerUri,
			config.AVDBootloaderInstallerFilename,
			config.SaltMaster,
		) + "\n")
	}

	// Restart system
	scriptBuilder.WriteString(GenerateRestartScript() + "\n")

	// End script
	scriptBuilder.WriteString(GenerateScriptEnd() + "\n")

	return scriptBuilder.String()
}

//go:embed 0_scriptStart.ps1
var scriptStart string
func GenerateScriptStart() string {
    return scriptStart
}

//go:embed 1_joinDomain.ps1
var joinDomainTemplate string
func GenerateJoinDomainScript(domainName, domainUsername, domainPassword, organizationalUnitPath string) string {
	script := joinDomainTemplate

	replacements := map[string]string{
		"$DOMAIN_NAME":              domainName,
		"$DOMAIN_USERNAME":          domainUsername,
		"$DOMAIN_PASSWORD":          domainPassword,
		"$ORGANIZATIONAL_UNIT_PATH": organizationalUnitPath,
	}

	for key, value := range replacements {
		script = strings.ReplaceAll(script, key, value)
	}

	return script
}

//go:embed 2_installAVD.ps1
var installAvdTemplate string
func GenerateInstallAvdScript(azureContainerUri, avdAgentInstallerFilename, avdBootloaderInstallerFilename, registrationToken string) string {
	script := installAvdTemplate

	replacements := map[string]string{
		"$AZURE_CONTAINER_URI":               azureContainerUri,
		"$AVD_AGENT_INSTALLER_FILENAME":      avdAgentInstallerFilename,
		"$AVD_BOOTLOADER_INSTALLER_FILENAME": avdBootloaderInstallerFilename,
		"$REGISTRATION_TOKEN":                registrationToken,
	}

	for key, value := range replacements {
		script = strings.ReplaceAll(script, key, value)
	}

	return script
}

//go:embed 3_installSaltMinion.ps1
var installSaltMinionTemplate string
func GenerateInstallSaltMinionScript(azureContainerUri, avdBootloaderInstallerFilename, saltMaster string) string {
	script := installSaltMinionTemplate

	// Replace placeholders with actual values
	replacements := map[string]string{
		"$AZURE_CONTAINER_URI":            azureContainerUri,
		"$SALT_MASTER":                    saltMaster,
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