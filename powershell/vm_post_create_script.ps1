# Set up logging for verbose output
$logFilePath = "$pwd\install_log.txt"
Start-Transcript -Path $logFilePath -Append

param (
    [string]$REGISTRATIONTOKEN,
    [string]$DOMAIN_NAME,
    [string]$DOMAIN_USERNAME,
    [string]$DOMAIN_PASSWORD,
    [bool]$CUSTOM_OU,
    [string]$OU_PATH,
    [string]$RDAgentURI = "https://query.prod.cms.rt.microsoft.com/cms/api/am/binary/RWrmXv",
    [string]$BootLoaderURI = "https://query.prod.cms.rt.microsoft.com/cms/api/am/binary/RWrxrH"
)

# Check if the machine is already in the domain
$computerDomain = (Get-WmiObject -Class Win32_ComputerSystem).Domain
if ($computerDomain -eq $DOMAIN_NAME) {
    Write-Host "Machine is already part of the domain: $DOMAIN_NAME"
} else {
    try {
        Write-Host "Attempting to join the domain: $DOMAIN_NAME"
        $securePassword = ConvertTo-SecureString -String $DOMAIN_PASSWORD -AsPlainText -Force
        $credential = New-Object System.Management.Automation.PSCredential ($DOMAIN_USERNAME, $securePassword)
        
        if ($CUSTOM_OU -eq $true -and $OU_PATH -ne "") {
            Write-Host "Joining with OU: $OU_PATH"
            Add-Computer -DomainName $DOMAIN_NAME -Credential $credential -OUPath $OU_PATH -Force -Verbose
        } else {
            Write-Host "Joining without specifying an OU"
            Add-Computer -DomainName $DOMAIN_NAME -Credential $credential -Force -Verbose
        }

        Write-Host "Successfully joined the domain."
    } catch {
        Write-Host "Error joining the domain: $_"
        Stop-Transcript
        exit 1
    }
}

# Define URLs for the installers (allows overriding)
$uris = @($RDAgentURI, $BootLoaderURI)

$installers = @()

# Download installers
foreach ($uri in $uris) {
	try {
		Write-Host "Starting download: $uri"
		$download = Invoke-WebRequest -Uri $uri -UseBasicParsing -Verbose
		$fileName = ($download.Headers.'Content-Disposition').Split('=')[1].Replace('"','')
		$outputPath = "$pwd\$fileName"

		if (Test-Path $outputPath) {
			Write-Host "File $fileName already exists. Skipping download."
		} else {
			$output = [System.IO.FileStream]::new($outputPath, [System.IO.FileMode]::Create)
			$output.write($download.Content, 0, $download.RawContentLength)
			$output.close()
			Write-Host "Downloaded: $fileName"
		}

		$installers += $outputPath
	} catch {
		Write-Host "Error downloading ${uri}: $_"
		return
	}
}

# Unblock files after download
foreach ($installer in $installers) {
	if (Test-Path $installer) {
		Write-Host "Unblocking file: $installer"
		Unblock-File -Path $installer -Verbose
	} else {
		Write-Host "File $installer not found, skipping unblock."
	}
}

# Find the RDAgent installer
$rdaAgentInstaller = $installers | Where-Object { $_ -match "Microsoft.RDInfra.RDAgent.Installer-x64" }
if (-not $rdaAgentInstaller) {
	Write-Host "RDAgent installer not found."
	return
}

# Find the BootLoader installer
$rdaBootLoaderInstaller = $installers | Where-Object { $_ -match "Microsoft.RDInfra.RDAgentBootLoader.Installer-x64" }
if (-not $rdaBootLoaderInstaller) {
	Write-Host "BootLoader Agent installer not found."
	return
}

# Install RDAgent
Write-Host "Installing RDAgent with registration token."
try {
	Start-Process msiexec -ArgumentList "/i $rdaAgentInstaller REGISTRATIONTOKEN=$REGISTRATIONTOKEN /quiet /norestart" -Wait -Verbose -Verb RunAs
} catch {
	Write-Host "Error installing RDAgent: $_"
	return
}

# Install BootLoader Agent
Write-Host "Installing BootLoader Agent."
try {
	Start-Process msiexec -ArgumentList "/i $rdaBootLoaderInstaller /quiet" -Wait -Verbose -Verb RunAs
} catch {
	Write-Host "Error installing BootLoader Agent: $_"
	return
}

# Finalize and restart
Write-Host "Preparing for system restart."
Restart-Computer -Force

Stop-Transcript