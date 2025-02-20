# --------------------------------------------------------------------------------
# Base script
# --------------------------------------------------------------------------------
$logFilePath = "$PSScriptRoot\install_log.txt"
Start-Transcript -Path $logFilePath -Append

function Exit-OnFailure {
    param([string]$message)
    Write-Host "ERROR: $message" -ForegroundColor Red
    Stop-Transcript
    exit 1
}