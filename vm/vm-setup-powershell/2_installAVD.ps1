# --------------------------------------------------------------------------------
# DOWNLOAD INSTALLERS FROM AZURE STORAGE
# --------------------------------------------------------------------------------
Write-Host "Downloading files from Azure Storage..."

$filesToDownload = @(
    $AVD_AGENT_INSTALLER_FILENAME,
    $AVD_BOOTLOADER_INSTALLER_FILENAME
)

$downloadFolder = Join-Path $env:TEMP "ArkloudDownloads"
if (!(Test-Path $downloadFolder)) {
    New-Item -ItemType Directory -Path $downloadFolder | Out-Null
}

foreach ($blobName in $filesToDownload) {
    Write-Host "Processing blob: $blobName"

    # Construct direct blob URL
    if ($AZURE_CONTAINER_URI -match "\?") {
        # If container URI already has '?', insert the blob name before the query string.
        $blobUrl = $AZURE_CONTAINER_URI -replace '\?.*$', "/$blobName$&"
    } else {
        # No query in base URI, just append the blob name.
        $blobUrl = "$AZURE_CONTAINER_URI/$blobName"
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
# INSTALL AVD AGENT + BOOTLOADER
# --------------------------------------------------------------------------------
Write-Host "AVD Agent and BootLoader installation starting..."

$rdaAgentInstallerPath      = Join-Path $downloadFolder $AVD_AGENT_INSTALLER_FILENAME
$rdaBootLoaderInstallerPath = Join-Path $downloadFolder $AVD_BOOTLOADER_INSTALLER_FILENAME

if (!(Test-Path $rdaAgentInstallerPath)) {
    Exit-OnFailure "Could not find $AVD_AGENT_INSTALLER_FILENAME in $downloadFolder"
}
if (!(Test-Path $rdaBootLoaderInstallerPath)) {
    Exit-OnFailure "Could not find $AVD_BOOTLOADER_INSTALLER_FILENAME in $downloadFolder"
}

Write-Host "Installing AVD RDAgent..."
try {
    Unblock-File -Path $rdaAgentInstallerPath
    $rdArgs = @("/i", $rdaAgentInstallerPath, "REGISTRATIONTOKEN=$REGISTRATION_TOKEN", "/quiet", "/norestart")
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
