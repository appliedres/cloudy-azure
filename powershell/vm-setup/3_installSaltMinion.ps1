# --------------------------------------------------------------------------------
# DOWNLOAD SALT MINION INSTALLER FROM AZURE BLOB STORAGE
# --------------------------------------------------------------------------------
Write-Host "Downloading Salt Minion installer from Azure Storage..."

$filesToDownload = @(
    $AVD_BOOTLOADER_INSTALLER_FILENAME,
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
Write-Host "Salt Minion installer downloaded successfully."


# --------------------------------------------------------------------------------
# INSTALL SALT MINION (MSI) & START SERVICE
# --------------------------------------------------------------------------------
Write-Host "Salt Minion installation starting..."

$saltInstallerPath = Join-Path $downloadFolder $SALT_MINION_INSTALLER_FILENAME
if (!(Test-Path $saltInstallerPath)) {
    Exit-OnFailure "Could not find $SALT_MINION_INSTALLER_FILENAME in $downloadFolder"
}

Write-Host "Installing Salt Minion (MSI) from local file: $saltInstallerPath"
Unblock-File -Path $saltInstallerPath

# Prepare MSI installation arguments
$saltArgs = @("/i", "$saltInstallerPath", "/quiet", "/norestart")
if (-not [string]::IsNullOrWhiteSpace($SALT_MASTER)) {
    Write-Host "Master specified during Salt Minion install: $SALT_MASTER"
    $saltArgs += "MASTER=$SALT_MASTER"
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
