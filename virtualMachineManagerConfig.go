package cloudyazure

type VirtualMachineManagerConfig struct {
	DomainControllers []*string
	SubnetIds         []string
	VnetId            string
}

type AzureVirtualDesktopConfig struct {
	AvdUsersGroupId              string
	DomainName                   string
	DomainUser                   string
	DomainPass                   string
	Region                       string
	DesktopApplicationUserRoleID string
	UriEnv                       string
	UriVersion                   string
	UseMultipleMonitors          string
	PrefixBase                   string
	HostPoolNamePrefix           string
	WorkspaceNamePrefix          string
	AppGroupNamePrefix           string
}
