Param(
    [int]$InstallTimeoutSeconds = 600  # Default timeout in seconds
)

# Ensure persistent log directory
$logDir = "C:\ProgramData\arkloud-bootstrap"
if (-not (Test-Path $logDir)) {
    New-Item -ItemType Directory -Path $logDir -Force | Out-Null
}

$logFilePath = Join-Path $logDir "install_log.txt"
Start-Transcript -Path $logFilePath -Append

function Exit-OnFailure {
    param([string]$message)
    Write-Host "ERROR: $message" -ForegroundColor Red
    Stop-Transcript
    exit 1
}

function Install-MSIWithRetry {
    param(
        [Parameter(Mandatory)][string]$InstallerPath,
        [Parameter(Mandatory)][string[]]$ArgumentList,
        [int]$TimeoutSeconds,
        [string]$Description = "Installer"
    )

    Write-Host "Starting $Description installation with retry logic (timeout $TimeoutSeconds seconds)..."
    $startTime = Get-Date

    while ($true) {
        $elapsed = (Get-Date) - $startTime
        if ($elapsed.TotalSeconds -ge $TimeoutSeconds) {
            Exit-OnFailure "$Description install timed out after $TimeoutSeconds seconds."
        }

        try {
            $process = Start-Process -FilePath "msiexec.exe" -ArgumentList $ArgumentList -NoNewWindow -PassThru -Wait

            switch ($process.ExitCode) {
                0      { Write-Host "$Description installation completed successfully."; return }
                3010   { Write-Host "$Description installed successfully (restart required)."; return }
                1618   { Write-Host "$Description install pending. Another installation is in progress. Retrying in 10 seconds..."; Start-Sleep -Seconds 10 }
                default { Exit-OnFailure "$Description installer exited with code $($process.ExitCode)." }
            }
        } catch {
            Write-Host "Encountered error starting $Description installer: $_. Retrying in 10 seconds..."
            Start-Sleep -Seconds 10
        }
    }
}
