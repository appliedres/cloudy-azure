package cloudyazure

type VirtualMachineManagerConfig struct {
	DomainControllers []*string

	SubnetIds []string

	VnetId string

}

type AzureVirtualDesktopConfig struct {
	AvdUsersGroupId string
	DomainName string
	DomainUser string
	DomainPass string
}