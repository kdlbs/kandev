#Requires -Version 5.1
<#
.SYNOPSIS
  Terminate exactly ONE kandev instance on Windows: the backend listening on a
  given port, plus its agentctl child(ren) and matching Vite dev server.

.DESCRIPTION
  Windows/PowerShell equivalent of the Unix-only scripts/kandev-kill. It NEVER
  does a blanket pkill. It resolves the backend PID from the port, confirms the
  process is actually a kandev backend, checks the recorded process start times,
  computes the exact set of PIDs it will terminate, PRINTS that set, and only
  then (after confirmation) stops them.

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
  Pidfile written by dev-isolated.ps1. Its sidecars record process start times
  and the Vite PID. Stale or missing web identity is never stopped by PID alone.

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
    $result += $current
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

function Stop-VerifiedProcess {
  param([pscustomobject]$ProcessRecord)
  $info = Get-ProcessInfo -TargetPid $ProcessRecord.Pid
  if ($info -and $info.CreationDate -eq $ProcessRecord.Created) {
    Stop-Process -Id $ProcessRecord.Pid -Force -ErrorAction SilentlyContinue
  }
}

function Test-AnyVerifiedProcessAlive {
  param([array]$ProcessRecords)
  foreach ($record in $ProcessRecords) {
    $info = Get-ProcessInfo -TargetPid $record.Pid
    if ($info -and $info.CreationDate -eq $record.Created) { return $true }
  }
  return $false
}

function Test-AnyPidAlive {
  param([array]$ProcessRecords)
  foreach ($record in $ProcessRecords) {
    if (Test-PidAlive -TargetPid $record.Pid) { return $true }
  }
  return $false
}

# --- Resolve the target backend PID + port ---
$backendPid = $null
$resolvedByPidfile = [bool]$Pidfile

if ($Port) {
  if ((Test-GuardedPort -Port $Port) -and -not $Force) {
    Write-Fail "kandev-kill: port $Port is guarded. Refusing without -Force."
    exit 3
  }
  $backendPid = Get-PidForPort -Port $Port
  if (-not $backendPid) {
    Write-Fail "kandev-kill: no listening process found on port $Port"
    exit 1
  }
  $candidatePidfile = Join-Path $env:TEMP ("kandev-dev-isolated-$Port.pid")
  if (Test-Path -LiteralPath $candidatePidfile) {
    $candidatePid = (Get-Content -LiteralPath $candidatePidfile -Raw).Trim()
    if ($candidatePid -eq "$backendPid") { $Pidfile = $candidatePidfile }
  }
} elseif ($Pidfile) {
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
} else {
  Write-Host 'kandev-kill: refusing to run without an explicit <port> or -Pidfile.' -ForegroundColor Red
  Write-Host 'Usage: scripts\kandev-kill.ps1 <port> [-Yes] [-Force]'
  Write-Host '       scripts\kandev-kill.ps1 -Pidfile <path> [-Yes]'
  exit 2
}

# --- Check the backend identity before collecting any process to stop ---
$backendInfo = Get-ProcessInfo -TargetPid $backendPid
$targetPorts = @(Get-ListeningPortsForPid -TargetPid $backendPid)
$expectedPort = $null
if ($Pidfile -and (Split-Path -Leaf $Pidfile) -match '^kandev-dev-isolated-([0-9]+)\.pid$') {
  $expectedPort = [int]$Matches[1]
}
$backendStartedFile = if ($Pidfile) { $Pidfile -replace '\.pid$', '.backend.started' } else { $null }
$homeFile = if ($Pidfile) { $Pidfile -replace '\.pid$', '.home' } else { $null }
$webPidfile = if ($Pidfile) { $Pidfile -replace '\.pid$', '.web.pid' } else { $null }
$webStartedFile = if ($Pidfile) { $Pidfile -replace '\.pid$', '.web.started' } else { $null }

if ($expectedPort -and (Test-GuardedPort -Port $expectedPort) -and -not $Force) {
  Write-Fail "kandev-kill: pidfile names guarded port $expectedPort. Refusing without -Force."
  exit 3
}
foreach ($listeningPort in $targetPorts) {
  if ((Test-GuardedPort -Port $listeningPort) -and -not $Force) {
    Write-Fail "kandev-kill: backend PID $backendPid listens on guarded port $listeningPort."
    exit 3
  }
}
if ($backendInfo) {
  if (-not (Test-KandevBackend -TargetPid $backendPid)) {
    Write-Fail "kandev-kill: PID $backendPid is not a kandev backend. Refusing to stop it."
    exit 4
  }
  if ($backendStartedFile -and (Test-Path -LiteralPath $backendStartedFile)) {
    $recordedStart = (Get-Content -LiteralPath $backendStartedFile -Raw).Trim()
    if ($recordedStart -notmatch '^[0-9]+$' -or
      $backendInfo.CreationDate.ToUniversalTime().Ticks -ne [long]$recordedStart) {
      Write-Fail "kandev-kill: backend PID $backendPid was reused. Refusing to stop it."
      exit 4
    }
  } elseif ($resolvedByPidfile -and ($targetPorts.Count -eq 0 -or ($expectedPort -and $targetPorts -notcontains $expectedPort))) {
    Write-Fail "kandev-kill: cannot verify backend PID $backendPid against its recorded port."
    exit 4
  }
}

# --- Compute the termination set: backend, then a separately verified web tree ---
$displayPort = if ($Port) { $Port } elseif ($expectedPort) { $expectedPort } elseif ($targetPorts.Count) { $targetPorts[0] } else { '?' }
$killSet = @()
if ($backendInfo) {
  $backendTree = @(Get-DescendantPids -RootPid $backendPid)
  if (-not $backendTree.Count -or $backendTree[0].Created -ne $backendInfo.CreationDate) {
    Write-Fail "kandev-kill: backend PID $backendPid changed while reading its process tree."
    exit 4
  }
  $killSet += $backendTree
}
$webPid = $null
$unverifiedWeb = $false
if ($webPidfile -and (Test-Path -LiteralPath $webPidfile)) {
  $webRaw = (Get-Content -LiteralPath $webPidfile -Raw).Trim()
  if ($webRaw -match '^[0-9]+$') {
    $webPid = [int]$webRaw
    $webInfo = Get-ProcessInfo -TargetPid $webPid
    if ($webInfo) {
      $recordedWebStart = if ($webStartedFile -and (Test-Path -LiteralPath $webStartedFile)) {
        (Get-Content -LiteralPath $webStartedFile -Raw).Trim()
      } else { '' }
      if ($recordedWebStart -match '^[0-9]+$' -and
        $webInfo.CreationDate.ToUniversalTime().Ticks -eq [long]$recordedWebStart -and
        $webInfo.Name -ieq 'node.exe' -and
        $webInfo.CommandLine -match 'node_modules[\\/]+vite[\\/]+bin[\\/]+vite\.js') {
        $webTree = @(Get-DescendantPids -RootPid $webPid)
        if (-not $webTree.Count -or $webTree[0].Created -ne $webInfo.CreationDate) {
          Write-Fail "kandev-kill: web PID $webPid changed while reading its process tree."
          exit 4
        }
        $killSet += $webTree
      } else {
        $unverifiedWeb = $true
        Write-Warning "kandev-kill: web PID $webPid has no matching launch identity; leaving it running."
      }
    }
  }
}
$killSet = @($killSet | Sort-Object Pid -Unique)

foreach ($record in $killSet) {
  foreach ($listeningPort in @(Get-ListeningPortsForPid -TargetPid $record.Pid)) {
    if ((Test-GuardedPort -Port $listeningPort) -and -not $Force) {
      Write-Fail "kandev-kill: PID $($record.Pid) listens on guarded port $listeningPort."
      exit 3
    }
  }
}

Write-Host "kandev-kill: will terminate the following processes (backend port $displayPort):"
foreach ($record in $killSet) {
  $info = Get-ProcessInfo -TargetPid $record.Pid
  if ($info) {
    Write-Host "  PID $($record.Pid)  $($info.Name)"
  } else {
    Write-Host "  PID $($record.Pid)  <gone>"
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
foreach ($record in $killSet) { Stop-VerifiedProcess -ProcessRecord $record }

$waited = 0
while ((Test-AnyPidAlive -ProcessRecords $killSet) -and ($waited -lt $GraceSeconds)) {
  Start-Sleep -Seconds 1
  $waited = $waited + 1
}
foreach ($record in $killSet) { Stop-VerifiedProcess -ProcessRecord $record }

if (Test-AnyVerifiedProcessAlive -ProcessRecords $killSet) {
  Write-Fail 'kandev-kill: a verified instance process is still running.'
  exit 5
}
if ($unverifiedWeb) { exit 4 }

if ($Pidfile) {
  Remove-Item -LiteralPath $Pidfile, $backendStartedFile, $homeFile, $webPidfile, $webStartedFile -Force -ErrorAction SilentlyContinue
}

Write-Host "kandev-kill: done. Backend on port $displayPort (PID $backendPid) terminated."
