package vm

type VirtualMachineManagerConfig struct {
	DomainControllers  []*string
	SubnetIds          []string
	VnetResourceGroup  string
	VnetId             string
}