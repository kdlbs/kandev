param(
  [Parameter(Mandatory = $true)]
  [string]$Path
)

$ErrorActionPreference = "Stop"

if ($env:WINDOWS_SIGNING_ENABLED -eq "false" -or $env:ALLOW_UNSIGNED_DESKTOP -eq "true") {
  Write-Warning "Skipping Windows signing for unsigned desktop artifact."
  exit 0
}

$missing = @()
if ([string]::IsNullOrWhiteSpace($env:WINDOWS_CERTIFICATE)) { $missing += "WINDOWS_CERTIFICATE" }
if ([string]::IsNullOrWhiteSpace($env:WINDOWS_CERTIFICATE_PASSWORD)) { $missing += "WINDOWS_CERTIFICATE_PASSWORD" }

if ($missing.Count -gt 0) {
  throw "Public Windows desktop releases require signing inputs: $($missing -join ', ')"
}

if (!(Test-Path $Path)) {
  throw "Cannot sign missing Windows artifact: $Path"
}

$certificatePath = Join-Path $env:RUNNER_TEMP "kandev-code-signing.pfx"
[IO.File]::WriteAllBytes($certificatePath, [Convert]::FromBase64String($env:WINDOWS_CERTIFICATE))

$timestampUrl = if ([string]::IsNullOrWhiteSpace($env:WINDOWS_TIMESTAMP_URL)) {
  "https://timestamp.digicert.com"
} else {
  $env:WINDOWS_TIMESTAMP_URL
}

$signTool = if ([string]::IsNullOrWhiteSpace($env:WINDOWS_SIGNTOOL_PATH)) {
  "signtool"
} else {
  $env:WINDOWS_SIGNTOOL_PATH
}

function Invoke-SignTool {
  param(
    [Parameter(Mandatory = $true)]
    [string[]]$Arguments,
    [Parameter(Mandatory = $true)]
    [string]$Description
  )

  & $signTool @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw "$Description failed with exit code $LASTEXITCODE"
  }
}

try {
  Invoke-SignTool `
    -Description "signtool sign" `
    -Arguments @(
      "sign",
      "/fd", "SHA256",
      "/td", "SHA256",
      "/tr", $timestampUrl,
      "/f", $certificatePath,
      "/p", $env:WINDOWS_CERTIFICATE_PASSWORD,
      $Path
    )
  Invoke-SignTool `
    -Description "signtool verify" `
    -Arguments @("verify", "/pa", $Path)
} finally {
  if (Test-Path -LiteralPath $certificatePath) {
    Remove-Item -LiteralPath $certificatePath -Force -ErrorAction SilentlyContinue
  }
}
