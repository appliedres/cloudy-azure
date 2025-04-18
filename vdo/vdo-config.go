package vdo

import (
	"github.com/appliedres/cloudy-azure/avd"
	"github.com/appliedres/cloudy-azure/vm"
)

type VirtualDesktopOrchestratorConfig struct {
	VM                              vm.VirtualMachineManagerConfig
	AVD                             *AVDConfig                    // optional, nil disables AVD install / configuration
	AD                              *ADJoinConfig                 // optional, nil disables AD join
	SaltMinionInstall               *SaltMinionInstallConfig      // optional, nil disables Salt Minion install
	BinaryStorage                   *InstallerBinaryStorageConfig // optional, nil disables all software installation (AVD / Salt Minion)
	RestartVirtualMachineAfterSetup bool                          // Whether to restart the VM after setup
}

// ADJoinConfig defines the settings required for Active Directory Join
type ADJoinConfig struct {
	DomainName             string
	DomainUsername         string
	DomainPassword         string
	OrganizationalUnitPath *string // optional, nil is not specified
}

// SaltMinionInstallConfig defines the settings required for Salt Minion installation
type SaltMinionInstallConfig struct {
	SaltMaster string // ip or hostname of Salt Master, to be used for registering the Salt Minion

	SaltMinionMsiFilename string // For windows installs

	SaltMinionRpmFilename string // For RHEL
	SaltCommonRpmFilename string

	SaltMinionDebFilename   string // For debian based, e.g. Ubuntu
	SaltCommonDebFilename   string
	BsdmainutilsDebFilename string
	DctrlToolsDebFilename   string
	NcalDebFilename         string
}

// InstallerBinaryStorageConfig defines the settings required for storing installer binaries
type InstallerBinaryStorageConfig struct {
	BlobStorageAccount string
	BlobContainer      string
}

type AVDConfig struct {
	InstallerConfig  AVDInstallConfig
	AVDManagerConfig avd.AzureVirtualDesktopManagerConfig
}

// AVDInstallConfig defines the settings required for AVD installation
type AVDInstallConfig struct {
	AVDAgentInstallerFilename      string
	AVDBootloaderInstallerFilename string
}

func isAVDEnabled(vdoConfig VirtualDesktopOrchestratorConfig) bool {
	if vdoConfig.AVD == nil {
		return false
	}

	return true
}