#Requires -Version 5.1
<#
.SYNOPSIS
  Launch a parallel, fully isolated kandev instance on Windows for debugging,
  without touching the user's live instance or production data.

.DESCRIPTION
  Windows/PowerShell equivalent of the Unix-only scripts/dev-isolated. It
  auto-picks non-colliding ports (scanning upward from a base and always
  skipping the well-known production ports), creates a throwaway KANDEV_HOME_DIR
  (fresh SQLite DB), builds the backend binaries if needed, launches the backend
  (and optionally the Vite dev frontend) with the `dev` profile + mock providers,
  waits for health, writes a pidfile, and prints the URLs, log paths, and the
  exact teardown command.

  A running kandev instance needs more than the `kandev` binary: the backend
  spawns `agentctl` to create executions, and the dev/mock profile launches
  `mock-agent`. This script ensures all of those exist before launching.

  Build strategy: when a (re)build is needed we run `make -C apps/backend build`,
  the canonical build path, with the correct flags/ldflags.

.PARAMETER Web
  Also start the Vite dev frontend (default: backend only).

.PARAMETER BackendPort
  Force the backend port (default: auto from base 48429).

.PARAMETER WebPort
  Force the web port (default: auto from base 47429).

.PARAMETER WebHost
  Host interface the Vite dev server binds to (default: 127.0.0.1). Pass a
  different host, such as 0.0.0.0, only when remote access is required.

.PARAMETER AgentctlPort
  Force the agentctl base port (default: auto from base 49429).

.PARAMETER NoBuild
  Do not (re)build; require kandev + agentctl + mock-agent to already exist.

.PARAMETER Install
  Run `make install` (backend deps + pnpm install + playwright browsers) before
  building/launching, for clean checkouts.

.PARAMETER CopyDb
  Seed the isolated DB from an existing kandev.db (copied, never the original)
  instead of starting empty.

.PARAMETER Timeout
  Health-wait timeout in seconds (default: 60).

.PARAMETER HomeDir
  Isolated KANDEV_HOME_DIR (default: %USERPROFILE%\.kandev-test). The script
  refuses a value that resolves to the real user profile root, the production
  ~/.kandev home, a drive/filesystem root, or a git workspace root.

.EXAMPLE
  scripts\dev-isolated.ps1

.EXAMPLE
  scripts\dev-isolated.ps1 -Web -Timeout 120

.NOTES
  Teardown (printed again at the end):
    scripts\kandev-kill.ps1 -Pidfile <pidfile> -Yes
#>
[CmdletBinding()]
param(
  [switch]$Web,
  [switch]$NoBuild,
  [switch]$Install,
  [string]$CopyDb,
  [int]$Timeout = 60,
  [int]$BackendPort,
  [int]$WebPort,
  [string]$WebHost = '127.0.0.1',
  [int]$AgentctlPort,
  [string]$HomeDir
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Split-Path -Parent $ScriptDir
$BackendDir = Join-Path $RepoRoot 'apps\backend'
$BinDir = Join-Path $BackendDir 'bin'
$BackendBin = Join-Path $BinDir 'kandev.exe'
$AgentctlBin = Join-Path $BinDir 'agentctl.exe'
$MockAgentBin = Join-Path $BinDir 'mock-agent.exe'
$NodeModules = Join-Path $RepoRoot 'apps\node_modules'
$RequiredBins = @($BackendBin, $AgentctlBin, $MockAgentBin)

# Ports we must never reuse (the well-known production ports).
$GuardedPorts = @(38429, 37429, 39429)

# Isolated defaults — deliberately far from production.
$BackendBase = 48429
$WebBase = 47429
$AgentctlBase = 49429

function Test-GuardedPort {
  param([int]$Port)
  return ($GuardedPorts -contains $Port)
}

function Test-PortInUse {
  param([int]$Port)
  $conn = Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue
  return ($null -ne $conn)
}

# Find a free port at or above $Base, never returning a guarded production port.
function Select-FreePort {
  param([int]$Base)
  $p = $Base
  while ($true) {
    if (-not (Test-GuardedPort -Port $p) -and -not (Test-PortInUse -Port $p)) {
      return $p
    }
    $p = $p + 1
    if ($p -gt ($Base + 500)) {
      throw "dev-isolated: could not find a free port near $Base"
    }
  }
}

# Reject an explicit port that would collide with the guarded production ports.
function Assert-NotGuarded {
  param([int]$Port, [string]$What)
  if (Test-GuardedPort -Port $Port) {
    throw "dev-isolated: $What port $Port is a guarded production port; refusing to launch an isolated instance on it."
  }
}

# Fail closed when -HomeDir resolves to a boundary that holds live state.
# Returns the resolved absolute path; refuses the real user profile root, the
# production ~/.kandev home, a drive/filesystem root, or a git workspace root.
function Resolve-SafeIsolatedHome {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [string]$ProfileRoot = $env:USERPROFILE,
    [string]$ProductionHome = (Join-Path $env:USERPROFILE '.kandev')
  )
  if ([string]::IsNullOrWhiteSpace($Path)) {
    throw 'dev-isolated: refusing an empty isolated home.'
  }
  $resolved = [System.IO.Path]::GetFullPath($Path).TrimEnd('\', '/')
  $profile = [System.IO.Path]::GetFullPath($ProfileRoot).TrimEnd('\', '/')
  $production = [System.IO.Path]::GetFullPath($ProductionHome).TrimEnd('\', '/')
  $driveRoot = [System.IO.Path]::GetPathRoot($resolved).TrimEnd('\', '/')
  $reasons = New-Object System.Collections.Generic.List[string]
  if ($resolved -eq $profile) { $reasons.Add('the real user profile root') }
  if ($resolved -eq $production) { $reasons.Add('the production kandev home') }
  if ($resolved -eq $driveRoot) { $reasons.Add('a drive or filesystem root') }
  if ((Test-Path -LiteralPath (Join-Path $resolved '.git')) -or (Test-Path -LiteralPath (Join-Path $resolved '.git\HEAD'))) {
    $reasons.Add('a git workspace root')
  }
  if ($reasons.Count -gt 0) {
    throw "dev-isolated: refusing -HomeDir '$resolved' because it is $($reasons -join ', '). Pass a dedicated isolated directory such as '$profile\.kandev-test'."
  }
  return $resolved
}

if ($BackendPort) { Assert-NotGuarded -Port $BackendPort -What 'backend' }
if ($WebPort) { Assert-NotGuarded -Port $WebPort -What 'web' }
if ($AgentctlPort) { Assert-NotGuarded -Port $AgentctlPort -What 'agentctl' }

# --- Fail closed: the isolated home must not be a live-state boundary ---
$RequestedHome = if ($HomeDir) { $HomeDir } else { Join-Path $env:USERPROFILE '.kandev-test' }
$IsolatedHome = Resolve-SafeIsolatedHome -Path $RequestedHome

$EffectiveBackendPort = if ($BackendPort) { $BackendPort } else { Select-FreePort -Base $BackendBase }
$EffectiveWebPort = if ($WebPort) { $WebPort } else { Select-FreePort -Base $WebBase }
$EffectiveAgentctlPort = if ($AgentctlPort) { $AgentctlPort } else { Select-FreePort -Base $AgentctlBase }

# Reserve an agentctl instance range above the base for spawned agent processes.
$AgentctlRangeBase = $EffectiveAgentctlPort + 100
$AgentctlRangeMax = $AgentctlRangeBase + 99

# --- Preflight: prerequisites with actionable errors ---
if (-not $NoBuild) {
  $makeCmd = Get-Command make -ErrorAction SilentlyContinue
  if (-not $makeCmd) {
    throw "dev-isolated: 'make' not found on PATH - install GNU Make (winget install ezwinports.make) before building, or pass -NoBuild if the binaries already exist."
  }
  $goCmd = Get-Command go -ErrorAction SilentlyContinue
  if (-not $goCmd) {
    throw "dev-isolated: 'go' not found on PATH - install Go or activate your toolchain (for example ``mise``) before building, or pass -NoBuild."
  }
}

# --- Optional clean-checkout setup ---
if ($Install) {
  Write-Host "dev-isolated: running 'make install' (backend deps + pnpm install + playwright browsers)..."
  & make -C $RepoRoot install
  if ($LASTEXITCODE -ne 0) { throw "dev-isolated: 'make install' failed (exit $LASTEXITCODE)." }
  Write-Host "dev-isolated: install complete."
}

# --- Ensure the backend binaries a live instance needs ---
function Test-RequiredBinsPresent {
  foreach ($bin in $RequiredBins) {
    if (-not (Test-Path -LiteralPath $bin)) { return $false }
  }
  return $true
}

if ($NoBuild) {
  if (-not (Test-RequiredBinsPresent)) {
    $missing = $RequiredBins | Where-Object { -not (Test-Path -LiteralPath $_) }
    $list = $missing -join [Environment]::NewLine
    throw "dev-isolated: -NoBuild set but required binaries are missing:`n$list`nBuild them with 'make -C apps/backend build' (or drop -NoBuild)."
  }
} else {
  $needBuild = $false
  if (-not (Test-RequiredBinsPresent)) {
    $needBuild = $true
  } else {
    # Rebuild if any Go source is newer than the OLDEST required binary.
    $oldest = ($RequiredBins | ForEach-Object { Get-Item -LiteralPath $_ } | Sort-Object LastWriteTime | Select-Object -First 1).LastWriteTime
    $newer = Get-ChildItem -LiteralPath $BackendDir -Recurse -Filter '*.go' -File -ErrorAction SilentlyContinue |
      Where-Object { $_.LastWriteTime -gt $oldest } |
      Select-Object -First 1
    if ($newer) { $needBuild = $true }
  }
  if ($needBuild) {
    Write-Host "dev-isolated: building backend binaries (make -C apps/backend build)..."
    & make -C $BackendDir build
    if ($LASTEXITCODE -ne 0) { throw "dev-isolated: build failed (exit $LASTEXITCODE)." }
    Write-Host "dev-isolated: build complete."
    if (-not (Test-RequiredBinsPresent)) {
      throw "dev-isolated: expected kandev.exe, agentctl.exe and mock-agent.exe after build, but one is missing."
    }
  }
}

# --- Preflight: web prerequisites ---
if ($Web -and -not (Test-Path -LiteralPath $NodeModules)) {
  throw "dev-isolated: node_modules not found - run 'make install' first, or pass -Install."
}

# --- Create the isolated home / data dir ---
$DataDir = Join-Path $IsolatedHome 'data'
New-Item -ItemType Directory -Path $DataDir -Force | Out-Null

# Minimal gitconfig so the isolated HOME does not prompt for identity.
$GitConfigPath = Join-Path $IsolatedHome '.gitconfig'
$GitConfig = @'
[user]
  name = Kandev Debug
  email = debug@kandev.local
[commit]
  gpgsign = false
[tag]
  gpgsign = false
'@
Set-Content -LiteralPath $GitConfigPath -Value $GitConfig -Encoding ASCII

$DbPath = Join-Path $DataDir 'kandev.db'
if ($CopyDb) {
  if (-not (Test-Path -LiteralPath $CopyDb)) { throw "dev-isolated: -CopyDb source not found: $CopyDb" }
  Copy-Item -LiteralPath $CopyDb -Destination $DbPath -Force
  foreach ($suffix in @('-wal', '-shm')) {
    $sidecar = "$CopyDb$suffix"
    if (Test-Path -LiteralPath $sidecar) {
      Copy-Item -LiteralPath $sidecar -Destination "$DbPath$suffix" -Force
    }
  }
  Write-Host "dev-isolated: seeded isolated DB from $CopyDb"
}

$RunDir = Join-Path $env:TEMP ("kandev-isolated-" + $EffectiveBackendPort)
New-Item -ItemType Directory -Path $RunDir -Force | Out-Null
$BackendOutLog = Join-Path $RunDir 'backend.out.log'
$BackendErrLog = Join-Path $RunDir 'backend.err.log'
$WebOutLog = Join-Path $RunDir 'web.out.log'
$WebErrLog = Join-Path $RunDir 'web.err.log'
# An empty stdin file keeps the detached child from inheriting the caller's
# console/pipe handles; without it the launching shell can block until the
# backend exits.
$StdinFile = Join-Path $RunDir 'stdin.empty'
New-Item -ItemType File -Path $StdinFile -Force | Out-Null
$Pidfile = Join-Path $env:TEMP ("kandev-dev-isolated-" + $EffectiveBackendPort + '.pid')
$WebPidfile = $Pidfile -replace '\.pid$', '.web.pid'

# --- Launch the backend (detached, logs to file) ---
# KANDEV_DEBUG_DEV_MODE=true selects the `dev` profile (mock agent, pprof,
# feature flags) from profiles.yaml. We also force mock providers so the
# isolated instance needs no real GitHub/agents, and bind to loopback to avoid
# Windows Firewall prompts.
$BackendUrl = "http://127.0.0.1:$EffectiveBackendPort"
Write-Host "dev-isolated: starting backend on :$EffectiveBackendPort ..."

$savedEnv = @{}
$overrides = [ordered]@{
  'HOME'                          = $IsolatedHome
  'USERPROFILE'                   = $IsolatedHome
  'KANDEV_HOME_DIR'               = $IsolatedHome
  'KANDEV_DATABASE_PATH'          = $DbPath
  'KANDEV_SERVER_HOST'            = '127.0.0.1'
  'KANDEV_SERVER_PORT'            = "$EffectiveBackendPort"
  'KANDEV_WEB_INTERNAL_URL'       = "http://127.0.0.1:$EffectiveWebPort"
  'KANDEV_AGENT_STANDALONE_PORT'  = "$EffectiveAgentctlPort"
  'AGENTCTL_INSTANCE_PORT_BASE'   = "$AgentctlRangeBase"
  'AGENTCTL_INSTANCE_PORT_MAX'    = "$AgentctlRangeMax"
  'KANDEV_DEBUG_DEV_MODE'         = 'true'
  'KANDEV_MOCK_AGENT'             = 'true'
  'KANDEV_MOCK_GITHUB'            = 'true'
  'KANDEV_MOCK_JIRA'              = 'true'
  'KANDEV_MOCK_LINEAR'            = 'true'
  'KANDEV_LOG_LEVEL'              = $(if ($env:KANDEV_LOG_LEVEL) { $env:KANDEV_LOG_LEVEL } else { 'info' })
  'PATH'                          = "$BinDir;$env:PATH"
}
foreach ($key in $overrides.Keys) {
  $savedEnv[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
  [Environment]::SetEnvironmentVariable($key, $overrides[$key], 'Process')
}

try {
  $backendProc = Start-Process -FilePath $BackendBin -ArgumentList '__backend' `
    -WorkingDirectory $BackendDir -WindowStyle Hidden -PassThru `
    -RedirectStandardInput $StdinFile `
    -RedirectStandardOutput $BackendOutLog -RedirectStandardError $BackendErrLog
} finally {
  foreach ($key in $savedEnv.Keys) {
    [Environment]::SetEnvironmentVariable($key, $savedEnv[$key], 'Process')
  }
}
$BackendPid = $backendProc.Id
Set-Content -LiteralPath $Pidfile -Value $BackendPid -Encoding ASCII

# --- Wait for backend health ---
$HealthUrl = "$BackendUrl/api/v1/system/health"
$deadline = (Get-Date).AddSeconds($Timeout)
$healthy = $false
while ((Get-Date) -lt $deadline) {
  if ($backendProc.HasExited) {
    Write-Host "dev-isolated: backend exited early. Last log lines:" -ForegroundColor Red
    if (Test-Path -LiteralPath $BackendErrLog) { Get-Content -LiteralPath $BackendErrLog -Tail 30 }
    if (Test-Path -LiteralPath $BackendOutLog) { Get-Content -LiteralPath $BackendOutLog -Tail 30 }
    Remove-Item -LiteralPath $Pidfile -Force -ErrorAction SilentlyContinue
    throw "dev-isolated: backend exited before becoming healthy."
  }
  try {
    $resp = Invoke-WebRequest -Uri $HealthUrl -UseBasicParsing -TimeoutSec 2
    if ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500) { $healthy = $true; break }
  } catch {
    # Not ready yet.
  }
  Start-Sleep -Seconds 1
}

if (-not $healthy) {
  Write-Host "dev-isolated: backend did not become healthy within ${Timeout}s. Last log lines:" -ForegroundColor Red
  if (Test-Path -LiteralPath $BackendErrLog) { Get-Content -LiteralPath $BackendErrLog -Tail 30 }
  if (Test-Path -LiteralPath $BackendOutLog) { Get-Content -LiteralPath $BackendOutLog -Tail 30 }
  Stop-Process -Id $BackendPid -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath $Pidfile -Force -ErrorAction SilentlyContinue
  throw "dev-isolated: backend did not become healthy within ${Timeout}s."
}

# --- Optionally launch the web frontend (Vite dev) ---
$webStarted = $false
if ($Web) {
  Write-Host "dev-isolated: starting web (vite dev) on :$EffectiveWebPort ..."
  $pnpmCmd = Get-Command pnpm.exe -ErrorAction SilentlyContinue
  if (-not $pnpmCmd) { $pnpmCmd = Get-Command pnpm.cmd -ErrorAction SilentlyContinue }
  if (-not $pnpmCmd) { $pnpmCmd = Get-Command pnpm -ErrorAction SilentlyContinue }
  if (-not $pnpmCmd) { throw "dev-isolated: 'pnpm' not found on PATH - install it (corepack/mise) before using -Web." }
  $savedWebEnv = @{}
  $webOverrides = [ordered]@{
    'VITE_KANDEV_API_PORT' = "$EffectiveBackendPort"
    'KANDEV_API_BASE_URL'  = $BackendUrl
    'PORT'                 = "$EffectiveWebPort"
    'VITE_KANDEV_DEBUG'    = 'true'
  }
  foreach ($key in $webOverrides.Keys) {
    $savedWebEnv[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
    [Environment]::SetEnvironmentVariable($key, $webOverrides[$key], 'Process')
  }
  try {
    $appsDir = Join-Path $RepoRoot 'apps'
    $webProc = Start-Process -FilePath $pnpmCmd.Source `
      -ArgumentList '-C', $appsDir, '--filter', '@kandev/web', 'exec', 'vite', '--host', $WebHost `
      -WorkingDirectory $appsDir -WindowStyle Hidden -PassThru `
      -RedirectStandardInput $StdinFile `
      -RedirectStandardOutput $WebOutLog -RedirectStandardError $WebErrLog
  } finally {
    foreach ($key in $savedWebEnv.Keys) {
      [Environment]::SetEnvironmentVariable($key, $savedWebEnv[$key], 'Process')
    }
  }
  Set-Content -LiteralPath $WebPidfile -Value $webProc.Id -Encoding ASCII

  $webUrl = "http://127.0.0.1:$EffectiveWebPort"
  $webDeadline = (Get-Date).AddSeconds($Timeout)
  while ((Get-Date) -lt $webDeadline) {
    if ($webProc.HasExited) { break }
    try {
      $webResp = Invoke-WebRequest -Uri $webUrl -UseBasicParsing -TimeoutSec 2
      if ($webResp.StatusCode -ge 200 -and $webResp.StatusCode -lt 500) { $webStarted = $true; break }
    } catch {
      # Not ready yet.
    }
    Start-Sleep -Seconds 1
  }
  if (-not $webStarted) {
    Write-Host "dev-isolated: WARNING - web did not become ready within ${Timeout}s (backend is fine; see $WebErrLog)." -ForegroundColor Yellow
  }
}

# --- Summary ---
Write-Host ''
Write-Host '================ kandev dev-isolated: READY ================'
Write-Host "  backend URL : $BackendUrl   (PID $BackendPid)"
if ($webStarted) {
  Write-Host "  web URL     : http://127.0.0.1:$EffectiveWebPort"
} elseif ($Web) {
  Write-Host "  web URL     : http://127.0.0.1:$EffectiveWebPort   (NOT ready - check log)"
} else {
  Write-Host '  web URL     : (not started; pass -Web to launch the frontend)'
}
Write-Host "  agentctl    : http://127.0.0.1:$EffectiveAgentctlPort  (range $AgentctlRangeBase-$AgentctlRangeMax)"
Write-Host "  KANDEV_HOME : $IsolatedHome"
Write-Host "  DB          : $DbPath"
Write-Host "  backend log : $BackendErrLog"
if ($Web) { Write-Host "  web log     : $WebErrLog" }
Write-Host "  pidfile     : $Pidfile"
Write-Host ''
Write-Host "  Teardown    : scripts\kandev-kill.ps1 -Pidfile `"$Pidfile`" -Yes"
Write-Host '==========================================================='
