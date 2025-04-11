# --------------------------------------------------------------------------------
# DOWNLOAD INSTALLERS FROM AZURE STORAGE
# --------------------------------------------------------------------------------
$downloadFolder = Join-Path $env:TEMP "ArkloudDownloads"
if (!(Test-Path $downloadFolder)) {
    New-Item -ItemType Directory -Path $downloadFolder | Out-Null
}

# Download AVD Agent
$avdAgentInstallerPath = Join-Path $downloadFolder "avd-agent.msi"
Write-Host "Downloading AVD Agent..."
try {
    Invoke-WebRequest -Uri "$AZURE_AVD_AGENT_URL" -OutFile $avdAgentInstallerPath -UseBasicParsing
    Write-Host "Successfully downloaded AVD Agent to $avdAgentInstallerPath"
} catch {
    Exit-OnFailure "Failed to download AVD Agent. Error: $_"
}
Write-Host "AVD Agent installer downloaded successfully."

# Download AVD BootLoader
$avdBootLoaderInstallerPath = Join-Path $downloadFolder "avd-bootloader.msi"
Write-Host "Downloading AVD BootLoader..."
try {
    Invoke-WebRequest -Uri "$AZURE_AVD_BOOTLOADER_URL" -OutFile $avdBootLoaderInstallerPath -UseBasicParsing
    Write-Host "Successfully downloaded AVD BootLoader to $avdBootLoaderInstallerPath"
} catch {
    Exit-OnFailure "Failed to download AVD BootLoader. Error: $_"
}
Write-Host "AVD Bootloader installer downloaded successfully."

# --------------------------------------------------------------------------------
# INSTALL AVD AGENT + BOOTLOADER
# --------------------------------------------------------------------------------

if (!(Test-Path $avdAgentInstallerPath)) {
    Exit-OnFailure "Could not find AVD Agent installer in $downloadFolder"
}
if (!(Test-Path $avdBootLoaderInstallerPath)) {
    Exit-OnFailure "Could not find AVD BootLoader installer in $downloadFolder"
}

Wait-ForInstaller -timeoutSeconds $InstallTimeoutSeconds
Write-Host "Installing AVD RDAgent..."
try {
    Unblock-File -Path $avdAgentInstallerPath
    $rdArgs = @("/i", $avdAgentInstallerPath, "REGISTRATIONTOKEN=$REGISTRATION_TOKEN", "/quiet", "/norestart")
    Start-Process msiexec.exe -ArgumentList $rdArgs -Wait -Verbose -Verb RunAs
} catch {
    Exit-OnFailure "Error installing RDAgent: $_"
}
Write-Host "AVD Agent installation completed successfully."

Wait-ForInstaller -timeoutSeconds $InstallTimeoutSeconds
Write-Host "Installing AVD BootLoader Agent..."
try {
    Unblock-File -Path $avdBootLoaderInstallerPath
    $blArgs = @("/i", $avdBootLoaderInstallerPath, "/quiet", "/norestart")
    Start-Process msiexec.exe -ArgumentList $blArgs -Wait -Verbose -Verb RunAs
} catch {
    Exit-OnFailure "Error installing BootLoader Agent: $_"
}
Write-Host "AVD BootLoader installation completed successfully."
