package avd

type AzureVirtualDesktopManagerConfig struct {
	// required
	AvdUsersGroupId              string
	DesktopApplicationUserRoleID string
	UriEnv                       string
	UriVersion                   string
	UseMultipleMonitors          string
	PrefixBase                   string
	PersonalHostPoolNamePrefix   string
	PersonalWorkspaceNamePrefix  string
	PersonalAppGroupNamePrefix   string
	PooledHostPoolNamePrefix     string
	PooledWorkspaceNamePrefix    string
	PooledAppGroupNamePrefix     string

	// optional
	RDAgentURI        *string
	BootLoaderURI     *string
	DesktopNamePrefix *string
}
