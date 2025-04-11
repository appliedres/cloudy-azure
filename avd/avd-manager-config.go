package avd

type AzureVirtualDesktopManagerConfig struct {
	// required
	AvdUsersGroupId              string
	DomainName                   string
	DomainUser                   string
	DomainPass                   string
	DesktopApplicationUserRoleID string
	UriEnv                       string
	UriVersion                   string
	UseMultipleMonitors          string
	PrefixBase                   string
	HostPoolNamePrefix           string
	WorkspaceNamePrefix          string
	AppGroupNamePrefix           string
	SaltMaster                   string
	StorageAccountName           string
	ContainerName                string

	// optional
	OUPath                         *string
	RDAgentURI                     *string
	BootLoaderURI                  *string
	DesktopNamePrefix              *string
	AvdAgentInstallerFilename      *string
	AvdBootloaderInstallerFilename *string
	SaltMinionInstallerFilename    *string
}
