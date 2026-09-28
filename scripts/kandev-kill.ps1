#Requires -Version 5.1
<#
.SYNOPSIS
  Terminate exactly ONE kandev instance on Windows: the backend listening on a
  given port, plus its agentctl child(ren) and matching Vite dev server.

.DESCRIPTION
  Windows/PowerShell equivalent of the Unix-only scripts/kandev-kill. It NEVER
  does a blanket pkill. It resolves the backend PID from the port, confirms the
  process is actually a kandev backend, computes the exact set of PIDs it will
  terminate, PRINTS that set, and only then (after confirmation) stops them.

  Safety:
    - Refuses to act unless you explicitly pass a port or a -Pidfile.
    - GUARDED PORTS (the well-known production ports: 38429 backend, 37429 web,
      39429 agentctl) are protected: killing one
      requires an explicit -Force. The guard is applied to the RESOLVED backend
      ports, so it holds no matter HOW you reached it - by <port> OR via a
      -Pidfile whose process happens to listen on a guarded port.
    - Prints exactly what will be terminated and asks for confirmation, unless
      -Yes is given (for scripted teardown).

  Windows has no SIGTERM for console processes, so Stop-Process is used for the
  whole set; -Force is accepted for compatibility and stragglers are always
  force-stopped after the grace period.

.PARAMETER Port
  Backend port to terminate (positional).

.PARAMETER Pidfile
  Pidfile written by dev-isolated.ps1. A sibling '.web.pid' sidecar, when
  present, is included in the termination set.

.PARAMETER Yes
  Skip the confirmation prompt (for scripted teardown).

.PARAMETER Force
  Allow terminating a guarded production port. Use with care.

.EXAMPLE
  scripts\kandev-kill.ps1 48429 -Yes

.EXAMPLE
  scripts\kandev-kill.ps1 -Pidfile "$env:TEMP\kandev-dev-isolated-48429.pid" -Yes
#>
[cmdletBinding(DefaultParameterSetName = 'Port')]
param(
  [Parameter(ParameterSetName = 'Port', Position = 0)]
  [int]$Port,

  [Parameter(ParameterSetName = 'Pidfile')]
  [string]$Pidfile,

  [switch]$Yes,

  [switch]$Force
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
# Write to stderr without raising a PowerShell error record, so the explicit
# `exit N` that follows runs and returns the intended exit code.
function Write-Fail {
  param([string]$Message)
  [Console]::Error.WriteLine($Message)
}

# Ports we refuse to kill without -Force (the well-known production ports).
$GuardedPorts = @(38429, 37429, 39429)
$GraceSeconds = 5

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

function Get-ListeningPortsForPid {
  param([int]$TargetPid)
  $ports = Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue |
    Where-Object { $_.OwningProcess -eq $TargetPid } |
    ForEach-Object { [int]$_.LocalPort } |
    Sort-Object -Unique
  return @($ports)
}

function Get-ProcessInfo {
  param([int]$TargetPid)
  return Get-CimInstance Win32_Process -Filter "ProcessId = $TargetPid" -ErrorAction SilentlyContinue
}

function Test-KandevBackend {
  param([int]$TargetPid)
  $info = Get-ProcessInfo -TargetPid $TargetPid
  if ($null -eq $info) { return $false }
  if ($info.Name -ieq 'kandev.exe' -or $info.Name -ieq 'kandev') { return $true }
  if ($info.ExecutablePath -and ($info.ExecutablePath -match '(\\|/)kandev(\.exe)?$')) { return $true }
  if ($info.CommandLine -and ($info.CommandLine -match 'kandev')) { return $true }
  return $false
}

function Get-DescendantPids {
  param([int]$RootPid)
  $all = Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
    Select-Object ProcessId, ParentProcessId, CreationDate
  $byId = @{}
  foreach ($p in $all) { $byId[[int]$p.ProcessId] = $p }

  # Windows keeps a dead process's ParentProcessId on its orphaned children, so
  # a reused PID makes an unrelated orphan look like our child. A genuine child
  # is created after its parent, so reject any candidate that predates it.
  $result = @()
  $queue = New-Object System.Collections.Queue
  $rootCreated = if ($byId.ContainsKey($RootPid)) { $byId[$RootPid].CreationDate } else { [datetime]::MinValue }
  $queue.Enqueue([pscustomobject]@{ Pid = $RootPid; Created = $rootCreated })
  while ($queue.Count -gt 0) {
    $current = $queue.Dequeue()
    $result += $current.Pid
    foreach ($child in $all) {
      if ([int]$child.ParentProcessId -ne $current.Pid) { continue }
      if (-not $child.CreationDate -or $child.CreationDate -lt $current.Created) { continue }
      $queue.Enqueue([pscustomobject]@{ Pid = [int]$child.ProcessId; Created = $child.CreationDate })
    }
  }
  return $result
}

function Test-PidAlive {
  param([int]$TargetPid)
  return ($null -ne (Get-Process -Id $TargetPid -ErrorAction SilentlyContinue))
}

# --- Resolve the target backend PID + port ---
$backendPid = $null
$resolvedByPidfile = $false

if ($Pidfile) {
  $resolvedByPidfile = $true
  if (-not (Test-Path -LiteralPath $Pidfile)) {
    Write-Fail "kandev-kill: pidfile not found: $Pidfile"
    exit 1
  }
  $raw = (Get-Content -LiteralPath $Pidfile -Raw).Trim()
  if ($raw -notmatch '^[0-9]+$') {
    Write-Fail "kandev-kill: pidfile has no numeric PID: $Pidfile"
    exit 1
  }
  $backendPid = [int]$raw
  if (-not (Test-PidAlive -TargetPid $backendPid)) {
    Write-Host "kandev-kill: process $backendPid from pidfile is already gone; nothing to do."
    Remove-Item -LiteralPath $Pidfile -Force -ErrorAction SilentlyContinue
    exit 0
  }
} elseif ($Port) {
  $backendPid = Get-PidForPort -Port $Port
  if (-not $backendPid) {
    Write-Fail "kandev-kill: no listening process found on port $Port"
    exit 1
  }
} else {
  Write-Host 'kandev-kill: refusing to run without an explicit <port> or -Pidfile.' -ForegroundColor Red
  Write-Host 'Usage: scripts\kandev-kill.ps1 <port> [-Yes] [-Force]'
  Write-Host '       scripts\kandev-kill.ps1 -Pidfile <path> [-Yes]'
  exit 2
}

# --- Resolve EVERY listening port of the target PID (for the guard) ---
$targetPorts = @(Get-ListeningPortsForPid -TargetPid $backendPid)

# Fail closed: in pidfile mode the guard relies entirely on resolving the
# target's listening ports. If none resolve and no port was supplied, we cannot
# prove the target is not a guarded production instance.
if ($resolvedByPidfile -and -not $Port -and $targetPorts.Count -eq 0 -and -not $Force) {
  Write-Fail "kandev-kill: cannot resolve listening ports for PID $backendPid. Refusing a pidfile kill without -Force."
  exit 3
}

$displayPort = if ($Port) { $Port } elseif ($targetPorts.Count -gt 0) { $targetPorts[0] } else { $null }

# --- Safety guards ---
$guardedHit = $null
if ($Port -and (Test-GuardedPort -Port $Port)) { $guardedHit = $Port }
if (-not $guardedHit) {
  foreach ($p in $targetPorts) {
    if (Test-GuardedPort -Port $p) { $guardedHit = $p; break }
  }
}
if ($guardedHit -and -not $Force) {
  Write-Fail "kandev-kill: port $guardedHit is a guarded production port. Refusing to kill PID $backendPid without -Force."
  exit 3
}

if (-not (Test-KandevBackend -TargetPid $backendPid)) {
  Write-Fail "kandev-kill: PID $backendPid does not look like a kandev backend. Aborting."
  exit 4
}

# --- Compute the termination set: backend + descendants (+ optional web pid) ---
$killSet = @(Get-DescendantPids -RootPid $backendPid)
if ($Pidfile) {
  $webPidfile = $Pidfile -replace '\.pid$', '.web.pid'
  if ($webPidfile -ne $Pidfile -and (Test-Path -LiteralPath $webPidfile)) {
    $webRaw = (Get-Content -LiteralPath $webPidfile -Raw).Trim()
    if ($webRaw -match '^[0-9]+$' -and (Test-PidAlive -TargetPid ([int]$webRaw))) {
      # The recorded web PID is the launcher wrapper; the Vite listener is a
      # descendant, so terminate the full tree rather than a single PID.
      $killSet += @(Get-DescendantPids -RootPid ([int]$webRaw))
    }
  }
}
$killSet = $killSet | Sort-Object -Unique

Write-Host "kandev-kill: will terminate the following processes (backend port $displayPort):"
foreach ($p in $killSet) {
  $info = Get-ProcessInfo -TargetPid $p
  if ($info) {
    $cmd = $info.CommandLine
    if ($cmd -and $cmd.Length -gt 90) { $cmd = $cmd.Substring(0, 90) }
    Write-Host "  PID $p  $cmd"
  } else {
    Write-Host "  PID $p  <gone>"
  }
}

if (-not $Yes) {
  $answer = Read-Host 'Proceed? [y/N]'
  if ($answer -notmatch '^(y|yes)$') {
    Write-Host 'kandev-kill: aborted by user.'
    exit 0
  }
}

# --- Terminate: stop the set, wait for the grace period, force stragglers. ---
foreach ($p in $killSet) {
  Stop-Process -Id $p -Force -ErrorAction SilentlyContinue
}

$waited = 0
while ((Test-PidAlive -TargetPid $backendPid) -and ($waited -lt $GraceSeconds)) {
  Start-Sleep -Seconds 1
  $waited = $waited + 1
}
if (Test-PidAlive -TargetPid $backendPid) {
  Write-Host "kandev-kill: backend still alive after ${GraceSeconds}s, forcing."
  foreach ($p in $killSet) {
    Stop-Process -Id $p -Force -ErrorAction SilentlyContinue
  }
  Start-Sleep -Seconds 1
}
foreach ($p in $killSet) {
  if (Test-PidAlive -TargetPid $p) {
    Stop-Process -Id $p -Force -ErrorAction SilentlyContinue
  }
}

if (Test-PidAlive -TargetPid $backendPid) {
  Write-Fail "kandev-kill: WARNING - backend PID $backendPid is still present."
  exit 5
}

if ($Pidfile) {
  Remove-Item -LiteralPath $Pidfile -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath ($Pidfile -replace '\.pid$', '.web.pid') -Force -ErrorAction SilentlyContinue
}

Write-Host "kandev-kill: done. Backend on port $displayPort (PID $backendPid) terminated."
