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
	AVDUserGroupID               string
	UriEnv                       string
	UriVersion                   string
	UseMulipleMonitors           string
	PrefixBase                   string
}
