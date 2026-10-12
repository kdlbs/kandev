#Requires -Version 5.1
$ErrorActionPreference = 'Stop'

Import-Module (Join-Path (Split-Path -Parent $PSScriptRoot) 'isolated-instance.psm1') -Force

$scriptsDir = Split-Path -Parent $PSScriptRoot
foreach ($scriptName in @('dev-isolated.ps1', 'kandev-kill.ps1', 'kandev-instances.ps1')) {
  $scriptPath = Join-Path $scriptsDir $scriptName
  $tokens = $null
  $parseErrors = $null
  [System.Management.Automation.Language.Parser]::ParseFile($scriptPath, [ref]$tokens, [ref]$parseErrors) | Out-Null
  if ($parseErrors.Count -gt 0) {
    throw "PowerShell parse errors in ${scriptName}: $($parseErrors -join '; ')"
  }
}

function Assert-True {
  param([bool]$Condition, [string]$Message)
  if (-not $Condition) { throw $Message }
}

function Assert-Throws {
  param([scriptblock]$Action, [string]$Message)
  try {
    & $Action
  } catch {
    return
  }
  throw $Message
}

function Assert-HomeRefused {
  param([scriptblock]$Action, [string]$Message)
  try {
    & $Action
  } catch {
    if ($_.Exception.Message -like 'dev-isolated: refusing -HomeDir*') { return }
    throw "${Message} Unexpected error: $($_.Exception.Message)"
  }
  throw $Message
}

$testRoot = Join-Path $env:TEMP ('kandev-isolated-script-test-' + [guid]::NewGuid().ToString('N'))
$profileRoot = Join-Path $testRoot 'profile'
$productionHome = Join-Path $profileRoot '.kandev'
$customProductionHome = Join-Path $testRoot 'custom-production-home'
$repoRoot = Join-Path $testRoot 'repo'
$junction = Join-Path $testRoot 'production-alias'
$dataJunction = Join-Path (Join-Path $profileRoot '.kandev-test-safe') 'data'
New-Item -ItemType Directory -Path $productionHome, $customProductionHome, $repoRoot -Force | Out-Null
New-Item -ItemType File -Path (Join-Path $repoRoot '.git') -Force | Out-Null

try {
  $safeHome = Join-Path $profileRoot '.kandev-test-safe'
  New-Item -ItemType Directory -Path $safeHome -Force | Out-Null
  $resolved = Resolve-SafeIsolatedHome -Path $safeHome -ProfileRoot $profileRoot `
    -LiveHomeRoots @($productionHome, $customProductionHome)
  Assert-True ($resolved -eq [System.IO.Path]::GetFullPath($safeHome).TrimEnd('\', '/')) `
    'A dedicated sibling home must be accepted.'

  Assert-HomeRefused {
    Resolve-SafeIsolatedHome -Path $profileRoot -ProfileRoot $profileRoot -LiveHomeRoots @($productionHome)
  } 'The user profile root must be refused.'
  Assert-HomeRefused {
    Resolve-SafeIsolatedHome -Path (Join-Path $productionHome 'data') -ProfileRoot $profileRoot `
      -LiveHomeRoots @($productionHome)
  } 'A child of the default production home must be refused.'
  Assert-HomeRefused {
    Resolve-SafeIsolatedHome -Path (Join-Path $customProductionHome 'nested') -ProfileRoot $profileRoot `
      -LiveHomeRoots @($customProductionHome)
  } 'A child of a configured production home must be refused.'
  Assert-HomeRefused {
    Resolve-SafeIsolatedHome -Path (Join-Path $repoRoot 'nested') -ProfileRoot $profileRoot
  } 'A path inside a Git worktree must be refused.'
  Assert-HomeRefused {
    Resolve-SafeIsolatedHome -Path ([System.IO.Path]::GetPathRoot($profileRoot)) -ProfileRoot $profileRoot
  } 'A drive root must be refused.'

  New-Item -ItemType Junction -Path $junction -Target $productionHome | Out-Null
  Assert-HomeRefused {
    Resolve-SafeIsolatedHome -Path (Join-Path $junction 'nested') -ProfileRoot $profileRoot `
      -LiveHomeRoots @($productionHome)
  } 'A path through a junction must be refused.'
  New-Item -ItemType Junction -Path $dataJunction -Target $customProductionHome | Out-Null
  Assert-HomeRefused {
    Assert-SafeIsolatedHomePaths -HomeDir $safeHome -Paths @((Join-Path $dataJunction 'kandev.db'))
  } 'A database path inside a reused home must not pass through a junction.'

  $originalVariables = @{
    KANDEV_DATABASE_DRIVER = $env:KANDEV_DATABASE_DRIVER
    KANDEV_INTERNAL_CONFIG_FILE = $env:KANDEV_INTERNAL_CONFIG_FILE
    DOCKER_HOST = $env:DOCKER_HOST
    VITE_SECRET_FOR_TEST = $env:VITE_SECRET_FOR_TEST
  }
  try {
    $env:KANDEV_DATABASE_DRIVER = 'postgres'
    $env:KANDEV_INTERNAL_CONFIG_FILE = Join-Path $testRoot 'live-config.yaml'
    $env:DOCKER_HOST = 'tcp://production-docker:2375'
    $env:VITE_SECRET_FOR_TEST = 'must-not-reach-vite'

    $isolatedConfig = Join-Path $testRoot 'isolated-config.yaml'
    $childEnvironment = New-KandevIsolatedChildEnvironment -BinaryDirectory $testRoot `
      -IsolatedHome $safeHome -Overrides @{
        KANDEV_DATABASE_PATH = (Join-Path $safeHome 'data\kandev.db')
        KANDEV_INTERNAL_CONFIG_FILE = $isolatedConfig
        TEMP = $testRoot
        TMP = $testRoot
      }
    $childSnapshot = Invoke-WithIsolatedEnvironment -Environment $childEnvironment -Action {
      [System.Environment]::GetEnvironmentVariables('Process')
    }

    Assert-True (-not $childSnapshot.Contains('KANDEV_DATABASE_DRIVER')) `
      'Inherited database driver settings must not reach the child.'
    Assert-True ($childSnapshot['KANDEV_INTERNAL_CONFIG_FILE'] -eq $isolatedConfig) `
      'The child must receive only the explicit isolated config file.'
    Assert-True (-not $childSnapshot.Contains('DOCKER_HOST')) `
      'Inherited Docker endpoints must not reach the child.'
    Assert-True (-not $childSnapshot.Contains('VITE_SECRET_FOR_TEST')) `
      'Inherited Vite variables must not reach the child.'
    Assert-True ($childSnapshot['KANDEV_HOME_DIR'] -eq $safeHome) `
      'The child must receive the isolated Kandev home.'
    Assert-True ($childSnapshot['APPDATA'] -eq (Join-Path $safeHome 'AppData\Roaming')) `
      'The child must receive isolated roaming application data.'
    Assert-True ($childSnapshot['LOCALAPPDATA'] -eq (Join-Path $safeHome 'AppData\Local')) `
      'The child must receive isolated local application data.'
    Assert-True ($childSnapshot['TEMP'] -eq $testRoot -and $childSnapshot['TMP'] -eq $testRoot) `
      'The child must receive only the explicit isolated temporary directory.'
    Assert-True ($childSnapshot['KANDEV_DATABASE_PATH'] -eq (Join-Path $safeHome 'data\kandev.db')) `
      'Explicit isolated settings must reach the child.'
    Assert-True ($childSnapshot['PATH'].StartsWith("$testRoot;")) `
      'The child PATH must start with the required binary directory.'
    Assert-True ($env:KANDEV_DATABASE_DRIVER -eq 'postgres') `
      'The caller environment must be restored after a successful action.'
  } finally {
    foreach ($key in $originalVariables.Keys) {
      [System.Environment]::SetEnvironmentVariable($key, $originalVariables[$key], 'Process')
    }
  }

  $originalDriver = $env:KANDEV_DATABASE_DRIVER
  $env:KANDEV_DATABASE_DRIVER = 'sqlite'
  try {
    Assert-Throws {
      Invoke-WithIsolatedEnvironment -Environment @{} -Action { throw 'expected test error' }
    } 'The environment wrapper must pass through action errors.'
    Assert-True ($env:KANDEV_DATABASE_DRIVER -eq 'sqlite') `
      'The caller environment must be restored after an action error.'
  } finally {
    [System.Environment]::SetEnvironmentVariable('KANDEV_DATABASE_DRIVER', $originalDriver, 'Process')
  }

  Write-Output 'Windows isolated instance safety tests passed.'
} finally {
  if (Test-Path -LiteralPath $junction) {
    [System.IO.Directory]::Delete($junction, $false)
  }
  if (Test-Path -LiteralPath $dataJunction) {
    [System.IO.Directory]::Delete($dataJunction, $false)
  }
  Remove-Item -LiteralPath $testRoot -Recurse -Force -ErrorAction SilentlyContinue
}
