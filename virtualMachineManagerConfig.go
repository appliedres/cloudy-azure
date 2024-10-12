package cloudyazure

type VirtualMachineManagerConfig struct {
	DomainControllers []*string

	SubnetIds []string

	VnetId string
}
