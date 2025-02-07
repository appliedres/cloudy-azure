# Designed to be run after the creation of a new Virtual Machine in Azure
#
# This script will:
# 1. Join the machine to an Active Directory domain
# 2. Download all files from a private Azure storage container
# 3. Install the AVD Agents (from the downloaded files)
# 4. Install the Salt Minion (using the downloaded bootstrap-salt.ps1)
# 5. Restart the machine

# Note: All values are inserted by the Go code at runtime via string replacement.
# These are replaced in a specific order, so be careful when editing this file.
$1_REGISTRATION_TOKEN                = "%s" # The registration token for the AVD Agent. Sourced from the target host pool.
$2_DOMAIN_NAME                       = "%s" # The domain name to join
$3_DOMAIN_USERNAME                   = "%s" # The username to use to join the domain
$4_DOMAIN_PASSWORD                   = "%s" # The password to use to join the domain
$5_OU_PATH                           = "%s" # The OU path to join the machine to (optional)
$6_SALT_MASTER                       = "%s" # The master to connect the Salt Minion to (optional)
$7_AZURE_CONTAINER_URI               = "%s" # e.g. "https://<acct>.blob.core.usgovcloudapi.net/container?sv=..."
$8_AVD_AGENT_INSTALLER_FILENAME      = "%s" # e.g. "Microsoft.RDInfra.RDAgent.Installer-x64.msi"
$9_AVD_BOOTLOADER_INSTALLER_FILENAME = "%s" # e.g. "Microsoft.RDInfra.RDAgentBootLoader.Installer-x64.msi"
$10_SALT_MINION_INSTALLER_FILENAME   = "%s" # e.g. "Salt-Minion-3006.7-Py3-AMD64-Setup.msi"

# --------------------------------------------------------------------------------
# Start Transcript
# --------------------------------------------------------------------------------
$logFilePath = "$PSScriptRoot\install_log.txt"
Start-Transcript -Path $logFilePath -Append

function Exit-OnFailure {
    param([string]$message)
    Write-Host "ERROR: $message" -ForegroundColor Red
    Stop-Transcript
    exit 1
}

# --------------------------------------------------------------------------------
# STEP 1: JOIN DOMAIN
# --------------------------------------------------------------------------------
Write-Host "Joining domain..."

$computerDomain = (Get-WmiObject -Class Win32_ComputerSystem).Domain
if ($computerDomain -eq "$2_DOMAIN_NAME") {
    Write-Host "Machine is already part of the domain: $2_DOMAIN_NAME"
} else {
    try {
        Write-Host "Attempting to join the domain: $2_DOMAIN_NAME"
        $securePassword = ConvertTo-SecureString -String "$4_DOMAIN_PASSWORD" -AsPlainText -Force
        $credential = New-Object System.Management.Automation.PSCredential ("$3_DOMAIN_USERNAME", $securePassword)

        if (-not [string]::IsNullOrWhiteSpace($5_OU_PATH)) {
            Write-Host "Joining with custom OU: $5_OU_PATH"
            Add-Computer -DomainName "$2_DOMAIN_NAME" -Credential $credential -OUPath "$5_OU_PATH" -Force -Verbose
        } else {
            Write-Host "Joining domain without specifying an OU"
            Add-Computer -DomainName "$2_DOMAIN_NAME" -Credential $credential -Force -Verbose
        }

        Write-Host "Successfully joined the domain."
    } catch {
        Exit-OnFailure "Error joining the domain: $_"
    }
}

# --------------------------------------------------------------------------------
# STEP 2: DOWNLOAD FILES FROM AZURE STORAGE
# --------------------------------------------------------------------------------
Write-Host "Downloading files from Azure Storage..."

$filesToDownload = @(
    $8_AVD_AGENT_INSTALLER_FILENAME,
    $9_AVD_BOOTLOADER_INSTALLER_FILENAME,
    $10_SALT_MINION_INSTALLER_FILENAME
)

$downloadFolder = Join-Path $env:TEMP "ArkloudDownloads"
if (!(Test-Path $downloadFolder)) {
    New-Item -ItemType Directory -Path $downloadFolder | Out-Null
}

foreach ($blobName in $filesToDownload) {
    Write-Host "Processing blob: $blobName"

    # Construct direct blob URL
    if ($7_AZURE_CONTAINER_URI -match "\?") {
        # If container URI already has '?', insert the blob name before the query string.
        $blobUrl = $7_AZURE_CONTAINER_URI -replace '\?.*$', "/$blobName$&"
    } else {
        # No query in base URI, just append the blob name.
        $blobUrl = "$7_AZURE_CONTAINER_URI/$blobName"
    }

    $outputPath = Join-Path $downloadFolder $blobName
    Write-Host "Downloading: $blobUrl -> $outputPath"

    try {
        Invoke-WebRequest -Uri $blobUrl -OutFile $outputPath -UseBasicParsing
        Write-Host "Successfully downloaded: $blobName"
    } catch {
        Exit-OnFailure "Failed to download file: $blobName. Error: $_"
    }
}
Write-Host "All files downloaded successfully."

# --------------------------------------------------------------------------------
# STEP 3: INSTALL AVD AGENT + BOOTLOADER
# --------------------------------------------------------------------------------
Write-Host "AVD Agent and BootLoader installation starting..."

$rdaAgentInstallerPath      = Join-Path $downloadFolder $8_AVD_AGENT_INSTALLER_FILENAME
$rdaBootLoaderInstallerPath = Join-Path $downloadFolder $9_AVD_BOOTLOADER_INSTALLER_FILENAME

if (!(Test-Path $rdaAgentInstallerPath)) {
    Exit-OnFailure "Could not find $8_AVD_AGENT_INSTALLER_FILENAME in $downloadFolder"
}
if (!(Test-Path $rdaBootLoaderInstallerPath)) {
    Exit-OnFailure "Could not find $9_AVD_BOOTLOADER_INSTALLER_FILENAME in $downloadFolder"
}

Write-Host "Installing AVD RDAgent..."
try {
    Unblock-File -Path $rdaAgentInstallerPath
    $rdArgs = @("/i", $rdaAgentInstallerPath, "REGISTRATIONTOKEN=$1_REGISTRATION_TOKEN", "/quiet", "/norestart")
    Start-Process msiexec.exe -ArgumentList $rdArgs -Wait -Verbose -Verb RunAs
} catch {
    Exit-OnFailure "Error installing RDAgent: $_"
}

Write-Host "Installing AVD BootLoader Agent..."
try {
    Unblock-File -Path $rdaBootLoaderInstallerPath
    $blArgs = @("/i", $rdaBootLoaderInstallerPath, "/quiet", "/norestart")
    Start-Process msiexec.exe -ArgumentList $blArgs -Wait -Verbose -Verb RunAs
} catch {
    Exit-OnFailure "Error installing BootLoader Agent: $_"
}
Write-Host "AVD Agent and BootLoader installation completed successfully."

# --------------------------------------------------------------------------------
# STEP 4: INSTALL SALT MINION (MSI) & START SERVICE
# --------------------------------------------------------------------------------
Write-Host "Salt Minion installation starting..."

$saltInstallerPath = Join-Path $downloadFolder $10_SALT_MINION_INSTALLER_FILENAME
if (!(Test-Path $saltInstallerPath)) {
    Exit-OnFailure "Could not find $10_SALT_MINION_INSTALLER_FILENAME in $downloadFolder"
}

Write-Host "Installing Salt Minion (MSI) from local file: $saltInstallerPath"
Unblock-File -Path $saltInstallerPath

# Prepare MSI installation arguments
$saltArgs = @("/i", "$saltInstallerPath", "/quiet", "/norestart")
if (-not [string]::IsNullOrWhiteSpace($6_SALT_MASTER)) {
    Write-Host "Master specified during salt minion install: $6_SALT_MASTER"
    $saltArgs += "MASTER=$6_SALT_MASTER"
}

Write-Host "Launching Salt Minion installer..."
try {
    $process = Start-Process -FilePath "msiexec.exe" -ArgumentList $saltArgs -NoNewWindow -PassThru -Wait
    if ($process.ExitCode -ne 0) {
        Exit-OnFailure "Salt Minion MSI did not exit cleanly. Exit code: $($process.ExitCode)"
    }
    Write-Host "Salt Minion MSI process exited successfully with code: $($process.ExitCode)"
} catch {
    Exit-OnFailure "Error launching Salt Minion installer: $_"
}

# Verify and start the salt-minion service
Write-Host "Verifying that the salt-minion service was registered..."
$service = $null
$tries = 0
$maxTries = 15
while (-not $service) {
    $service = Get-Service salt-minion -ErrorAction SilentlyContinue
    if ($service) { break }
    Start-Sleep -Seconds 2
    $tries++
    if ($tries -ge $maxTries) {
        Exit-OnFailure "Timeout waiting for 'salt-minion' service to be installed."
    }
}
Write-Host "'salt-minion' service found successfully."

# Start the salt-minion service if it's not already running
Write-Host "Starting salt-minion service..."
$serviceRefreshTime = 0
$serviceStartMax = 60
while ($service.Status -ne "Running") {
    if ($service.Status -eq "Stopped") {
        Start-Service -Name "salt-minion" -ErrorAction SilentlyContinue
    }
    Start-Sleep -Seconds 2
    $service.Refresh()
    if ($service.Status -eq "Running") {
        Write-Host "Salt-minion service started successfully."
        break
    } else {
        $serviceRefreshTime++
        if ($serviceRefreshTime -ge $serviceStartMax) {
            Exit-OnFailure "Timed out waiting for salt-minion service to start."
        }
    }
}
Write-Host "Salt Minion installation and service startup completed successfully!"

# --------------------------------------------------------------------------------
# STEP 5: RESTART
# --------------------------------------------------------------------------------
Write-Host "Preparing for system restart..."
Restart-Computer -Force

Stop-Transcript
