package cloudyazure

type VirtualMachineManagerConfig struct {
	DomainControllers []*string

	SubnetIds []string

	VnetId string

	AvdConfig AzureVirtualDesktopConfig
}

type AzureVirtualDesktopConfig struct {
	ConnectionTimeout int
	HostPoolsConfig []HostPoolsConfig
}

type HostPoolsConfig struct {
	HostPoolName string
	ConnectionType string
	MaxSessions int
}