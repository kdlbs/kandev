param(
  [Parameter(Mandatory = $true)]
  [string]$Path
)

$ErrorActionPreference = "Stop"

$missing = @()
if ([string]::IsNullOrWhiteSpace($env:WINDOWS_CERTIFICATE)) { $missing += "WINDOWS_CERTIFICATE" }
if ([string]::IsNullOrWhiteSpace($env:WINDOWS_CERTIFICATE_PASSWORD)) { $missing += "WINDOWS_CERTIFICATE_PASSWORD" }

if ($missing.Count -gt 0) {
  if ($env:ALLOW_UNSIGNED_DESKTOP -eq "true") {
    Write-Warning "Skipping Windows signing for internal unsigned desktop validation; missing: $($missing -join ', ')"
    exit 0
  }
  throw "Public Windows desktop releases require signing inputs: $($missing -join ', ')"
}

if (!(Test-Path $Path)) {
  throw "Cannot sign missing Windows artifact: $Path"
}

$certificatePath = Join-Path $env:RUNNER_TEMP "kandev-code-signing.pfx"
if (!(Test-Path $certificatePath)) {
  [IO.File]::WriteAllBytes($certificatePath, [Convert]::FromBase64String($env:WINDOWS_CERTIFICATE))
}

$timestampUrl = if ([string]::IsNullOrWhiteSpace($env:WINDOWS_TIMESTAMP_URL)) {
  "http://timestamp.digicert.com"
} else {
  $env:WINDOWS_TIMESTAMP_URL
}

$signTool = if ([string]::IsNullOrWhiteSpace($env:WINDOWS_SIGNTOOL_PATH)) {
  "signtool"
} else {
  $env:WINDOWS_SIGNTOOL_PATH
}

& $signTool sign /fd SHA256 /td SHA256 /tr $timestampUrl /f $certificatePath /p $env:WINDOWS_CERTIFICATE_PASSWORD $Path
& $signTool verify /pa $Path
