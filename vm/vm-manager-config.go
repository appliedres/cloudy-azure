package vm

type VirtualMachineManagerConfig struct {
	DomainControllers  []*string
	SubnetIds          []string
	VnetResourceGroup  string
	VnetId             string
	InitialSetupConfig *SetupScriptConfig
}

// SetupScriptConfig defines the overall configuration for the powershell script used in
// the initial setup process on the Virtual Machine.
// Marking a section nil / not defined will remove it from the setup process.
type SetupScriptConfig struct {
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
	OrganizationalUnitPath *string // optional, nil is not specified
}

// AVDInstallConfig defines the settings required for AVD installation
type AVDInstallConfig struct {
	AVDAgentInstallerFilename      string
	AVDBootloaderInstallerFilename string
}

// SaltMinionInstallConfig defines the settings required for Salt Minion installation

type SaltMinionInstallConfig struct {
	SaltMaster string // ip or hostname of Salt Master, to be used for registering the Salt Minion

	SaltMinionMsiFilename 	*string // For windows installs

	SaltMinionRpmFilename 	string // For RHEL
	SaltBaseRpmFilename 	string

	SaltMinionDebFilename   string // For debian based, e.g. Ubuntu
	SaltCommonDebFilename   string
	BsdmainDebFilename 		string
	BsdextraDebFilename     string
	DctrlToolsDebFilename   string
	NcalDebFilename         string
}

// InstallerBinaryStorageConfig defines the settings required for storing installer binaries
type InstallerBinaryStorageConfig struct {
	BlobStorageAccount string
	BlobContainer      string
}
