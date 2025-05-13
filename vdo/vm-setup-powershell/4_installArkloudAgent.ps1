# -------------------------------------------------------------------------
# CONFIGURE ARKLOUD ENDPOINT & API KEY
# -------------------------------------------------------------------------
param(
    [string]$ApiUrl = $env:ARKLOUD_API_URL,
    [string]$ApiKey = $env:ARKLOUD_API_KEY,
    [string]$agentExeUrl = $env:AZURE_ARKLOUD_AGENT_URL
)

if (-not $ApiUrl) {
    $ApiUrl = Read-Host "Enter Arkloud API URL (e.g. https://arkloud.example.com)"
}
if (-not $ApiKey) {
    $ApiKey = Read-Host "Enter Arkloud API Key" -AsSecureString | ConvertFrom-SecureString -AsPlainText
}

# 1) Set user-level environment variables (persist in HKCU\Environment)
try {
    $envPath = "HKCU:\Environment"
    Set-ItemProperty -Path $envPath -Name "ARKLOUD_API_URL" -Value $ApiUrl
    Set-ItemProperty -Path $envPath -Name "ARKLOUD_API_KEY" -Value $ApiKey
    # make them available to the current session too:
    [Environment]::SetEnvironmentVariable("ARKLOUD_API_URL", $ApiUrl, "Process")
    [Environment]::SetEnvironmentVariable("ARKLOUD_API_KEY", $ApiKey, "Process")
    Write-Host "Stored API URL and Key as user-level environment variables"
} catch {
    Exit-OnFailure "Failed to write environment variables: $_"
}

# 2) Write fallback files beside the agent exe
try {
    $installDir = "C:\Program Files\ArkloudAgent"
    Set-Content -Path (Join-Path $installDir "api_url.txt") -Value $ApiUrl -Encoding UTF8
    Set-Content -Path (Join-Path $installDir "api_key.txt") -Value $ApiKey -Encoding UTF8
    Write-Host "Wrote api_url.txt and api_key.txt for filesystem fallback"
} catch {
    Exit-OnFailure "Failed to write api_url.txt/api_key.txt: $_"
}

Write-Host "Arkloud API configuration saved.  Restart the agent (or log out/in) for changes to take effect."

# --------------------------------------------------------------------------------
# DOWNLOAD ARKLOUD AGENT EXECUTABLE FROM STORAGE
# --------------------------------------------------------------------------------
Write-Host "Downloading Arkloud Agent executable from Azure Storage..."

$downloadFolder = Join-Path $env:TEMP "ArkloudDownloads"
if (-not (Test-Path $downloadFolder)) {
    New-Item -ItemType Directory -Path $downloadFolder | Out-Null
}

# You must set $agentExeUrl to point at your arkloudagent.exe blob
$agentExePath = Join-Path $downloadFolder "arkloudagent.exe"

Write-Host "Downloading Arkloud Agent..."
try {
    Invoke-WebRequest -Uri "$agentExeUrl" -OutFile $agentExePath -UseBasicParsing
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
# REGISTER AUTO-START (STARTUP FOLDER + RUN KEY FALLBACK)
# --------------------------------------------------------------------------------
# 1) Create a shortcut in the user's Startup folder
$startupFolder = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\Startup"
$shortcutPath  = Join-Path $startupFolder "ArkloudAgent.lnk"

try {
    $shell    = New-Object -ComObject WScript.Shell
    $shortcut = $shell.CreateShortcut($shortcutPath)
    $shortcut.TargetPath     = Join-Path $installDir "arkloudagent.exe"
    $shortcut.WorkingDirectory = $installDir
    $shortcut.Save()
    Write-Host "Startup shortcut created at $shortcutPath"
} catch {
    Exit-OnFailure "Failed to create startup shortcut. Error: $_"
}

# 2) Also register the Run key as a fallback
try {
    $runKey = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
    Set-ItemProperty -Path $runKey -Name "ArkloudAgent" -Value "`"$installDir\arkloudagent.exe`""
    Write-Host "Registered ArkloudAgent in HKCU Run key"
} catch {
    Exit-OnFailure "Failed to write Run-key for ArkloudAgent. Error: $_"
}

# --------------------------------------------------------------------------------
# LAUNCH THE AGENT & VERIFY IT STARTS
# --------------------------------------------------------------------------------
Write-Host "Launching Arkloud Agent..."
try {
    $process = Start-Process -FilePath (Join-Path $installDir "arkloudagent.exe") -PassThru
} catch {
    Exit-OnFailure "Failed to start Arkloud Agent. Error: $_"
}

# Poll for the process to confirm startup
$maxTries = 15
$delay    = 2  # seconds
for ($i = 0; $i -lt $maxTries; $i++) {
    if (Get-Process -Id $process.Id -ErrorAction SilentlyContinue) {
        Write-Host "Arkloud Agent is running (PID $($process.Id))."
        break
    }
    Start-Sleep -Seconds $delay
    if ($i -eq $maxTries - 1) {
        Exit-OnFailure "Timed out waiting for Arkloud Agent to start."
    }
}

Write-Host "Arkloud Agent installation and startup completed successfully!"
