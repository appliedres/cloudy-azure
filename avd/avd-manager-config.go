package avd

type AzureVirtualDesktopConfig struct {
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

	// optional
	OUPath                       *string
	RDAgentURI                   *string
	BootLoaderURI				 *string
	SaltMaster					 *string
	DesktopNamePrefix            *string
}
