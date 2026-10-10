#Requires -Version 5.1
<#
.SYNOPSIS
  List running kandev backend instances on this Windows host.

.DESCRIPTION
  Windows/PowerShell companion to scripts/kandev-instances. For each backend
  process (a `kandev` executable with a listening TCP socket) it prints:

    PID  BACKEND_PORT  AGENTCTL_PORT  HOME_DIR  REPO_PATH  MARKER

  This is the tool a debugger uses to tell "my isolated instance" apart from
  "the user's live instance" before touching anything. A `PRODUCTION` marker is
  shown when a backend listens on one of the guarded production ports
  (38429/37429/39429).

  HOME_DIR is resolved from a sibling pidfile written by dev-isolated.ps1 when
  present; it is shown as `?` otherwise, because Windows does not expose another
  process's environment the way /proc does on Unix.

.PARAMETER Raw
  Print tab-separated values with no header or marker column (scriptable).

.EXAMPLE
  scripts\kandev-instances.ps1

.EXAMPLE
  scripts\kandev-instances.ps1 -Raw
#>
[cmdletBinding()]
param(
  [switch]$Raw
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$GuardedPorts = @(38429, 37429, 39429)

function Test-GuardedPort {
  param([int]$Port)
  return ($GuardedPorts -contains $Port)
}

function Get-PidForPort {
  param([int]$Port)
  $conn = Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue |
    Select-Object -First 1
  if ($null -eq $conn) { return $null }
  return [int]$conn.OwningProcess
}

# Emit "PID Port" pairs for every listening kandev backend socket, keeping the
# lowest port per PID (the configured backend HTTP port is the canonical one).
$pairs = @{}
$listeners = Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue
foreach ($conn in $listeners) {
  $pidValue = [int]$conn.OwningProcess
  $proc = Get-Process -Id $pidValue -ErrorAction SilentlyContinue
  if (-not $proc) { continue }
  if ($proc.ProcessName -ine 'kandev') { continue }
  $port = [int]$conn.LocalPort
  if (-not $pairs.ContainsKey($pidValue) -or $port -lt $pairs[$pidValue]) {
    $pairs[$pidValue] = $port
  }
}

if ($pairs.Count -eq 0) {
  if (-not $Raw) { Write-Host '(no running kandev instances found)' }
  exit 0
}

function Get-AgentctlPort {
  param([int]$BackendPid)
  $children = Get-CimInstance Win32_Process -Filter "ParentProcessId = $BackendPid" -ErrorAction SilentlyContinue
  foreach ($child in $children) {
    if ($child.CommandLine -and ($child.CommandLine -match 'agentctl') -and ($child.CommandLine -match 'port=([0-9]+)')) {
      return $Matches[1]
    }
  }
  return '?'
}

function Get-IsolatedHome {
  param([int]$BackendPid)
  $candidates = Get-ChildItem -LiteralPath $env:TEMP -Filter 'kandev-dev-isolated-*.pid' -File -ErrorAction SilentlyContinue
  foreach ($candidate in $candidates) {
    if ($candidate.Name -notmatch '^kandev-dev-isolated-[0-9]+\.pid$') { continue }
    $raw = (Get-Content -LiteralPath $candidate.FullName -Raw).Trim()
    if ($raw -eq "$BackendPid") {
      $startedFile = $candidate.FullName -replace '\.pid$', '.backend.started'
      if (-not (Test-Path -LiteralPath $startedFile)) { continue }
      $recordedStart = (Get-Content -LiteralPath $startedFile -Raw).Trim()
      $backendInfo = Get-CimInstance Win32_Process -Filter "ProcessId = $BackendPid" -ErrorAction SilentlyContinue
      if (-not $backendInfo -or $recordedStart -notmatch '^[0-9]+$' -or
        $backendInfo.CreationDate.ToUniversalTime().Ticks -ne [long]$recordedStart) { continue }
      $homeFile = $candidate.FullName -replace '\.pid$', '.home'
      if (-not (Test-Path -LiteralPath $homeFile)) { continue }
      $recordedHome = (Get-Content -LiteralPath $homeFile -Raw).Trim()
      if ($recordedHome) { return $recordedHome }
      continue
    }
  }
  return '?'
}

if (-not $Raw) {
  '{0,-7}  {1,-13}  {2,-13}  {3,-24}  {4}' -f 'PID', 'BACKEND_PORT', 'AGENTCTL_PORT', 'HOME_DIR', 'REPO_PATH'
}

$rows = foreach ($pidValue in ($pairs.Keys | Sort-Object)) {
  $backendPort = $pairs[$pidValue]
  $agentctlPort = Get-AgentctlPort -BackendPid $pidValue
  $homeDir = Get-IsolatedHome -BackendPid $pidValue
  $proc = Get-Process -Id $pidValue -ErrorAction SilentlyContinue
  $repoPath = '?'
  if ($proc -and $proc.Path -and ($proc.Path -match '(\\|/)apps(\\|/)backend(\\|/)bin(\\|/)kandev(\.exe)?$')) {
    $repoPath = Split-Path -Parent (Split-Path -Parent (Split-Path -Parent (Split-Path -Parent $proc.Path)))
  }
  $marker = if (Test-GuardedPort -Port $backendPort) { 'PRODUCTION' } else { 'isolated' }

  if ($Raw) {
    '{0}{1}{2}{3}{4}{5}{6}{7}{8}' -f $pidValue, "`t", $backendPort, "`t", $agentctlPort, "`t", $homeDir, "`t", $repoPath
  } else {
    '{0,-7}  {1,-13}  {2,-13}  {3,-24}  {4}  {5}' -f $pidValue, $backendPort, $agentctlPort, $homeDir, $repoPath, $marker
  }
}
if ($Raw) {
  $rows
} else {
  $rows | ForEach-Object { Write-Host $_ }
}
