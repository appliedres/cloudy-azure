package vm

type VirtualMachineManagerConfig struct {
	DomainControllers  []*string
	SubnetIds          []string
	VnetResourceGroup  string
	VnetId             string
	InitialSetupConfig *PowershellConfig
}

// PowershellConfig defines the overall configuration for the powershell script used in
// the initial setup process on the Virtual Machine.
// Marking a section nil / not defined will remove it from the setup process.
type PowershellConfig struct {
	ADJoin                  *ADJoinConfig
	AVDInstall              *AVDInstallConfig
	SaltMinionInstallConfig *SaltMinionInstallConfig
	BinaryStorage           *InstallerBinaryStorageConfig
	RestartVirtualMachine   bool
}

// ADJoinConfig defines the settings required for Active Directory Join
type ADJoinConfig struct {
	DomainName             string
	DomainUsername         string
	DomainPassword         string
	OrganizationalUnitPath *string  // optional, nil is not specified
}

// AVDInstallConfig defines the settings required for AVD installation
type AVDInstallConfig struct {
	AVDAgentInstallerFilename      string
	AVDBootloaderInstallerFilename string
}

// SaltMinionInstallConfig defines the settings required for Salt Minion installation
type SaltMinionInstallConfig struct {
	SaltMaster                  string // ip or hostname of Salt Master, to be used for registering the Salt Minion
	SaltMinionInstallerFilename string
}

// InstallerBinaryStorageConfig defines the settings required for storing installer binaries
type InstallerBinaryStorageConfig struct {
	BlobStorageAccount string
	BlobContainer      string
}
