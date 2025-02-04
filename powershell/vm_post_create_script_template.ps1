# Designed to be run after the creation of a new Virtual Machine in Azure
#
# This script will:
# 1. Join the machine to an Active Directory domain
# 2. Download and install the AVD Agents
# 3. Download and install the Salt Minion (optionally connect to a master)
#
# Note: All values are inserted by the Go code at runtime via string replacement.
# These are replaced in a specific order, so be careful when editing this file.
$REGISTRATION_TOKEN  = "%s" # The registration token for the AVD Agent. Sourced from the target host pool.
$DOMAIN_NAME         = "%s" # The domain name to join
$DOMAIN_USERNAME     = "%s" # The username to use to join the domain
$DOMAIN_PASSWORD     = "%s" # The password to use to join the domain
$OU_PATH             = "%s" # The OU path to join the machine to (optional)
$AVD_AGENT_URI       = "%s" # The URI for the AVD Agent
$AVD_BOOTLOADER_URI  = "%s" # The URI for the AVD Agent BootLoader
$SALT_MASTER         = "%s" # The master to connect the Salt Minion to (optional)

# Set up logging for verbose output
$logFilePath = "$pwd\install_log.txt"
Start-Transcript -Path $logFilePath -Append

function Exit-OnFailure {
    param([string]$message)
    Write-Host $message
    Stop-Transcript
    exit 1
}

# Step 1: Join Machine to Domain
$computerDomain = (Get-WmiObject -Class Win32_ComputerSystem).Domain
if ($computerDomain -eq "$DOMAIN_NAME") {
    Write-Host "Machine is already part of the domain: $DOMAIN_NAME"
} else {
    try {
        Write-Host "Attempting to join the domain: $DOMAIN_NAME"
        $securePassword = ConvertTo-SecureString -String "$DOMAIN_PASSWORD" -AsPlainText -Force
        $credential = New-Object System.Management.Automation.PSCredential ("$DOMAIN_USERNAME", $securePassword)

        if ("$OU_PATH" -ne "") {
            Write-Host "Joining with custom OU: $OU_PATH"
            Add-Computer -DomainName "$DOMAIN_NAME" -Credential $credential -OUPath "$OU_PATH" -Force -Verbose
        } else {
            Write-Host "Joining domain without specifying an OU"
            Add-Computer -DomainName "$DOMAIN_NAME" -Credential $credential -Force -Verbose
        }

        Write-Host "Successfully joined the domain."
    } catch {
        Exit-OnFailure "Error joining the domain: $_"
    }
}

# Step 2: Download & Install AVD Agents
$installers = @()
$uris = @("$AVD_AGENT_URI", "$AVD_BOOTLOADER_URI")

foreach ($uri in $uris) {
    try {
        Write-Host "Downloading: $uri"
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
        Exit-OnFailure "Error downloading $($uri): $($_)"
    }
}

# Step 3: Unblock & Install AVD Agents
foreach ($installer in $installers) {
    if (Test-Path $installer) {
        Write-Host "Unblocking: $installer"
        Unblock-File -Path $installer -Verbose
    } else {
        Exit-OnFailure "File $installer not found, skipping installation."
    }
}

$rdaAgentInstaller = $installers | Where-Object { $_ -match "Microsoft.RDInfra.RDAgent.Installer-x64" }
if (-not $rdaAgentInstaller) { Exit-OnFailure "RDAgent installer not found." }

$rdaBootLoaderInstaller = $installers | Where-Object { $_ -match "Microsoft.RDInfra.RDAgentBootLoader.Installer-x64" }
if (-not $rdaBootLoaderInstaller) { Exit-OnFailure "BootLoader Agent installer not found." }

Write-Host "Installing RDAgent..."
try {
    $rdArgs = @("/i", "$rdaAgentInstaller", "REGISTRATIONTOKEN=$REGISTRATION_TOKEN", "/quiet", "/norestart")
    Start-Process msiexec -ArgumentList $rdArgs -Wait -Verbose -Verb RunAs
} catch {
    Exit-OnFailure "Error installing RDAgent: $_"
}

Write-Host "Installing BootLoader Agent..."
try {
    $blArgs = @("/i", "$rdaBootLoaderInstaller", "/quiet")
    Start-Process msiexec -ArgumentList $blArgs -Wait -Verbose -Verb RunAs
} catch {
    Exit-OnFailure "Error installing BootLoader Agent: $_"
}

# Step 4: Download & Install Salt Minion using bootstrap-salt.ps1
try {
    Write-Host "Downloading Salt Bootstrap Script..."
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.SecurityProtocolType]'Tls12'
    Invoke-WebRequest -Uri https://github.com/saltstack/salt-bootstrap/releases/latest/download/bootstrap-salt.ps1 `
                      -OutFile "$env:TEMP\bootstrap-salt.ps1" -UseBasicParsing -Verbose
} catch {
    Exit-OnFailure "Error downloading bootstrap-salt.ps1: $_"
}

Write-Host "Running bootstrap-salt.ps1..."
try {
    # If a master is specified, pass -Master <master> to the script.
    $saltBootArgs = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "$env:TEMP\bootstrap-salt.ps1")
    if ($SALT_MASTER) {
        Write-Host "Using Salt Master: $SALT_MASTER"
        $saltBootArgs += ("-Master", $SALT_MASTER)
    }
    
    Start-Process powershell.exe -ArgumentList $saltBootArgs -Wait -Verbose
} catch {
    Exit-OnFailure "Error running bootstrap-salt.ps1: $_"
}

# Step 5: Verify Salt Minion is up
Write-Host "Verifying Salt Minion..."
try {
    $saltVersion = salt-call --version
    if ($saltVersion) {
        Write-Host "Salt Minion installed successfully: $saltVersion"
    } else {
        Exit-OnFailure "Salt Minion installation verification failed!"
    }
} catch {
    Write-Host "Could not run 'salt-call --version' in this session. The Minion service may still be running."
}

Write-Host "Salt Minion installation completed successfully!"

# Step 6: Restart the Computer (Optional if you want a clean reboot post-install)
Write-Host "Preparing for system restart..."
Restart-Computer -Force

Stop-Transcript
