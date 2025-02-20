# --------------------------------------------------------------------------------
# DOWNLOAD SALT MINION INSTALLER FROM AZURE STORAGE
# --------------------------------------------------------------------------------
Write-Host "Downloading Salt Minion installer from Azure Storage..."

$downloadFolder = Join-Path $env:TEMP "ArkloudDownloads"
if (!(Test-Path $downloadFolder)) {
    New-Item -ItemType Directory -Path $downloadFolder | Out-Null
}

# Define the Salt Minion installer path
$saltInstallerPath = Join-Path $downloadFolder "salt-minion.msi"

# Download Salt Minion installer
Write-Host "Downloading Salt Minion installer..."
try {
    Invoke-WebRequest -Uri "$AZURE_SALT_MINION_URL" -OutFile $saltInstallerPath -UseBasicParsing
    Write-Host "Successfully downloaded Salt Minion installer to $saltInstallerPath"
} catch {
    Exit-OnFailure "Failed to download Salt Minion installer. Error: $_"
}

Write-Host "Salt Minion installer downloaded successfully."

# --------------------------------------------------------------------------------
# INSTALL SALT MINION (MSI) & START SERVICE
# --------------------------------------------------------------------------------
Write-Host "Salt Minion installation starting..."

if (!(Test-Path $saltInstallerPath)) {
    Exit-OnFailure "Could not find Salt Minion installer in $downloadFolder"
}

Write-Host "Installing Salt Minion (MSI) from local file..."
Unblock-File -Path $saltInstallerPath

# Prepare MSI installation arguments
$saltArgs = @("/i", "$saltInstallerPath", "/quiet", "/norestart")
if (-not [string]::IsNullOrWhiteSpace("$SALT_MASTER")) {
    Write-Host "Master specified during Salt Minion install."
    $saltArgs += "MASTER=$SALT_MASTER"
}

Write-Host "Launching Salt Minion installer..."
try {
    $process = Start-Process -FilePath "msiexec.exe" -ArgumentList $saltArgs -NoNewWindow -PassThru -Wait
    if ($process.ExitCode -ne 0) {
        Exit-OnFailure "Salt Minion MSI did not exit cleanly. Exit code: $($process.ExitCode)"
    }
    Write-Host "Salt Minion MSI process exited successfully."
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
