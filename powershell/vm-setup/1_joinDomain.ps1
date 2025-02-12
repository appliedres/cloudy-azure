# --------------------------------------------------------------------------------
# STEP 1: JOIN DOMAIN
# --------------------------------------------------------------------------------
Write-Host "Joining domain..."

$computerDomain = (Get-WmiObject -Class Win32_ComputerSystem).Domain
if ($computerDomain -eq "$DOMAIN_NAME") {
    Write-Host "Machine is already part of the domain: $DOMAIN_NAME"
} else {
    try {
        Write-Host "Attempting to join the domain: $DOMAIN_NAME"
        $securePassword = ConvertTo-SecureString -String "$DOMAIN_PASSWORD" -AsPlainText -Force
        $credential = New-Object System.Management.Automation.PSCredential ("$DOMAIN_USERNAME", $securePassword)

        if (-not [string]::IsNullOrWhiteSpace($ORGANIZATIONAL_UNIT_PATH)) {
            Write-Host "Joining with custom OU: $ORGANIZATIONAL_UNIT_PATH"
            Add-Computer -DomainName "$DOMAIN_NAME" -Credential $credential -OUPath "$ORGANIZATIONAL_UNIT_PATH" -Force -Verbose
        } else {
            Write-Host "Joining domain without specifying an OU"
            Add-Computer -DomainName "$DOMAIN_NAME" -Credential $credential -Force -Verbose
        }

        Write-Host "Successfully joined the domain."
    } catch {
        Exit-OnFailure "Error joining the domain: $_"
    }
}
