# -------------------------------------------------------------------------
# CONFIGURE ARKLOUD ENDPOINT & API KEY
# -------------------------------------------------------------------------

$AZURE_ARKLOUD_AGENT_URL = "!!AZURE_ARKLOUD_AGENT_URL"
$ARKLOUD_API_URL         = "!!ARKLOUD_API_URL"
$ARKLOUD_API_KEY         = "!!ARKLOUD_API_KEY"

if (-not $AZURE_ARKLOUD_AGENT_URL) {
    Exit-OnFailure "Could not find AZURE_ARKLOUD_AGENT_URL. Please set it in the script."
}
if (-not $ARKLOUD_API_URL) {
    Exit-OnFailure "ARKLOUD_API_URL is not set. Please set it in the script."
}
if (-not $ARKLOUD_API_KEY) {
    Exit-OnFailure "ARKLOUD_API_KEY is not set. Please set it in the script."
}

# Save for every account on the machine
try {
    [Environment]::SetEnvironmentVariable("ARKLOUD_API_URL", $ARKLOUD_API_URL, "Machine")
    [Environment]::SetEnvironmentVariable("ARKLOUD_API_KEY", $ARKLOUD_API_KEY, "Machine")

    # Also set for the current process so the immediate launch inherits them
    [Environment]::SetEnvironmentVariable("ARKLOUD_API_URL", $ARKLOUD_API_URL, "Process")
    [Environment]::SetEnvironmentVariable("ARKLOUD_API_KEY", $ARKLOUD_API_KEY, "Process")

    Write-Host "Stored API URL and Key as machine-level environment variables"
} catch {
    Exit-OnFailure "Failed to write environment variables: $_"
}

Write-Host "Arkloud Agent configuration completed successfully!"

# --------------------------------------------------------------------------------
# DOWNLOAD ARKLOUD AGENT EXECUTABLE FROM STORAGE
# --------------------------------------------------------------------------------
Write-Host "Downloading Arkloud Agent executable from Azure Storage..."

$downloadFolder = Join-Path $env:TEMP "ArkloudDownloads"
if (-not (Test-Path $downloadFolder)) {
    New-Item -ItemType Directory -Path $downloadFolder | Out-Null
}

# You must set $AZURE_ARKLOUD_AGENT_URL to point at your arkloudagent.exe blob
$agentExePath = Join-Path $downloadFolder "arkloudagent.exe"

Write-Host "Downloading Arkloud Agent..."
try {
    Invoke-WebRequest -Uri "$AZURE_ARKLOUD_AGENT_URL" -OutFile $agentExePath -UseBasicParsing
    Write-Host "Successfully downloaded Arkloud Agent to $agentExePath"
} catch {
    Exit-OnFailure "Failed to download Arkloud Agent executable. Error: $_"
}

# --------------------------------------------------------------------------------
# INSTALL ARKLOUD AGENT & SETUP AUTO-START
# --------------------------------------------------------------------------------
Write-Host "Installing Arkloud Agent..."

$installDir = "C:\Program Files\ArkloudAgent"
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir | Out-Null
}

# Copy the executable into Program Files
try {
    Copy-Item -Path $agentExePath -Destination $installDir -Force
    Write-Host "Copied arkloudagent.exe to $installDir"
} catch {
    Exit-OnFailure "Failed to copy Arkloud Agent to $installDir. Error: $_"
}

# Unblock the executable so Windows won’t block it at runtime
Unblock-File -Path (Join-Path $installDir "arkloudagent.exe")

# --------------------------------------------------------------------------------
# REGISTER AUTO-START WITH WINDOWS SERVICE
# --------------------------------------------------------------------------------
$serviceName = "ArkloudAgent"
$exePath     = "`"$installDir\arkloudagent.exe`""

try {
    if (-not (Get-Service -Name $serviceName -ErrorAction SilentlyContinue)) {
        New-Service -Name $serviceName -BinaryPathName $exePath -DisplayName "Arkloud Agent" -StartupType Automatic
        Write-Host "Windows service '$serviceName' created"
    } else {
        sc.exe config $serviceName binPath= $exePath start= auto | Out-Null
        Write-Host "Windows service '$serviceName' updated"
    }
} catch {
    Exit-OnFailure "Failed to create or update Windows service. Error: $_"
}

# --------------------------------------------------------------------------------
# LAUNCH THE AGENT & VERIFY IT STARTS
# --------------------------------------------------------------------------------
Write-Host "Starting Arkloud Agent service..."
try {
    Start-Service -Name $serviceName
} catch {
    Exit-OnFailure "Failed to start Arkloud Agent service. Error: $_"
}

# Poll for the service to confirm it is running
$maxTries = 10
$delay    = 2  # seconds

for ($i = 0; $i -lt $maxTries; $i++) {
    $svc = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
    if ($svc -and $svc.Status -eq "Running") {
        Write-Host "Arkloud Agent service is running."
        break
    }

    Start-Sleep -Seconds $delay

    if ($i -eq $maxTries - 1) {
        Exit-OnFailure "Timed out waiting for Arkloud Agent service to reach the 'Running' state."
    }
}

Write-Host "Arkloud Agent installation and startup completed successfully!"