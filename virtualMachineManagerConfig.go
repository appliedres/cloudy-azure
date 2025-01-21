package cloudyazure

type VirtualMachineManagerConfig struct {
	DomainControllers []*string
	SubnetIds         []string
	VnetResourceGroup string
	VnetId            string
}

type AzureVirtualDesktopConfig struct {
	AvdUsersGroupId              string
	DomainName                   string
	DomainUser                   string
	DomainPass                   string
	OUPath						 *string
	DesktopApplicationUserRoleID string
	UriEnv                       string
	UriVersion                   string
	UseMultipleMonitors          string
	PrefixBase                   string
	HostPoolNamePrefix           string
	WorkspaceNamePrefix          string
	AppGroupNamePrefix           string
}
