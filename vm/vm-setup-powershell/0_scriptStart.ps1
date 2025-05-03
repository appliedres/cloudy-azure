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

function Get-InstallerNameFromInProgress {
    $inProgressKey = "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Installer\InProgress"
    if (Test-Path $inProgressKey) {
        try {
            $installerInfo = Get-ItemProperty -Path $inProgressKey
            if ($installerInfo.ProductName) {
                return $installerInfo.ProductName
            }
            elseif ($installerInfo.ProductCode) {
                return $installerInfo.ProductCode
            }
        } catch {
            # Ignore any errors, fallback if needed
        }
    }
    return $null
}

function Wait-ForInstaller {
    param([int]$timeoutSeconds)

    Write-Host "Checking for active Windows Installer processes..."
    $startTime = Get-Date
    $printedDetails = $false  # only print details once

    while ($true) {
        $elapsed = (Get-Date) - $startTime
        $elapsedSec = [int]$elapsed.TotalSeconds

        if ($elapsedSec -gt $timeoutSeconds) {
            Exit-OnFailure "Wait-ForInstaller timed out after $timeoutSeconds seconds."
        }

        $installerProcesses = Get-WmiObject Win32_Process | Where-Object { $_.Name -eq "msiexec.exe" }

        if ($installerProcesses) {
            # First time we detect msiexec, print a detailed block
            if (-not $printedDetails) {
                $printedDetails = $true

                $installerName = Get-InstallerNameFromInProgress

                # Build a single block of process details
                $procDetails = $installerProcesses | ForEach-Object {
                    "  PID: $($_.ProcessId); Cmd: $($_.CommandLine)"
                } | Out-String

                Write-Host "Another installation is in progress (msiexec)."
                Write-Host "Currently running processes:"
                Write-Host $procDetails

                if ($installerName) {
                    Write-Host "Detected product from InProgress registry key: $installerName"
                }
                else {
                    Write-Host "Unable to determine exactly what is being installed."
                }
            }

            Start-Sleep -Seconds 5
        }
        else {
            # No msiexec found, we're done waiting
            break
        }
    }

    # If we get here, no more msiexec processes are running
    $endTime = Get-Date
    $totalElapsed = ($endTime - $startTime).TotalSeconds
    $remaining = $timeoutSeconds - [int]$totalElapsed

    Write-Host ("Wait-ForInstaller finished after {0} seconds, with {1} seconds remaining out of the {2}-second timeout." -f 
        [int]$totalElapsed, $remaining, $timeoutSeconds)

    Write-Host "No active installations detected. Proceeding..."
}