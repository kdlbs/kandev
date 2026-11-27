#Requires -Version 5.1

function Get-NormalizedPath {
  param([Parameter(Mandatory = $true)][string]$Path)
  $fullPath = [System.IO.Path]::GetFullPath($Path)
  $root = [System.IO.Path]::GetPathRoot($fullPath)
  while ($fullPath.Length -gt $root.Length -and
    ($fullPath.EndsWith('\') -or $fullPath.EndsWith('/'))) {
    $fullPath = $fullPath.Substring(0, $fullPath.Length - 1)
  }
  return $fullPath
}

function Test-PathWithin {
  param([string]$Path, [string]$Root)
  $candidate = Get-NormalizedPath -Path $Path
  $boundary = Get-NormalizedPath -Path $Root
  if ([string]::Equals($candidate, $boundary, [System.StringComparison]::OrdinalIgnoreCase)) {
    return $true
  }
  $prefix = $boundary
  if (-not $prefix.EndsWith('\') -and -not $prefix.EndsWith('/')) { $prefix += '\' }
  return $candidate.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)
}

function Expand-IsolatedHomePath {
  param([string]$Path, [string]$ProfileRoot)
  if ($Path -eq '~') { return $ProfileRoot }
  if ($Path.StartsWith('~\') -or $Path.StartsWith('~/')) {
    return Join-Path $ProfileRoot $Path.Substring(2)
  }
  return $Path
}

function Get-PathItemOrNull {
  param([string]$Path, [string]$HomePath)
  try {
    return (Get-Item -LiteralPath $Path -Force -ErrorAction Stop)
  } catch {
    if ($_.CategoryInfo.Category -eq [System.Management.Automation.ErrorCategory]::ObjectNotFound -or
      $_.Exception -is [System.IO.FileNotFoundException] -or
      $_.Exception -is [System.IO.DirectoryNotFoundException]) {
      return $null
    }
    throw "dev-isolated: refusing -HomeDir '$HomePath' because it cannot verify path component '$Path': $($_.Exception.Message)"
  }
}

function Resolve-SafeIsolatedHome {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$ProfileRoot,
    [string[]]$LiveHomeRoots = @()
  )
  if ([string]::IsNullOrWhiteSpace($Path) -or [string]::IsNullOrWhiteSpace($ProfileRoot)) {
    throw 'dev-isolated: refusing an empty home path or profile root.'
  }

  $profile = Get-NormalizedPath -Path $ProfileRoot
  $requestedPath = Expand-IsolatedHomePath -Path $Path -ProfileRoot $profile
  $resolved = Get-NormalizedPath -Path $requestedPath
  $driveRoot = Get-NormalizedPath -Path ([System.IO.Path]::GetPathRoot($resolved))
  if ([string]::Equals($resolved, $driveRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "dev-isolated: refusing -HomeDir '$resolved' because it is a drive or filesystem root."
  }
  if ([string]::Equals($resolved, $profile, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "dev-isolated: refusing -HomeDir '$resolved' because it is the real user profile root."
  }

  $productionRoots = @((Join-Path $profile '.kandev')) + @($LiveHomeRoots)
  foreach ($root in $productionRoots) {
    if ([string]::IsNullOrWhiteSpace($root)) { continue }
    $normalizedRoot = Get-NormalizedPath -Path (Expand-IsolatedHomePath -Path $root -ProfileRoot $profile)
    if (Test-PathWithin -Path $resolved -Root $normalizedRoot) {
      throw "dev-isolated: refusing -HomeDir '$resolved' because it is inside a live Kandev home '$normalizedRoot'."
    }
  }

  $currentPath = $resolved
  while ($currentPath) {
    $item = Get-PathItemOrNull -Path $currentPath -HomePath $resolved
    if ($item) {
      if (-not $item.PSIsContainer) {
        throw "dev-isolated: refusing -HomeDir '$resolved' because its path passes through a file."
      }
      if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "dev-isolated: refusing -HomeDir '$resolved' because its path uses a symbolic link or junction."
      }
      if ($item.PSIsContainer) {
        $gitMetadata = Get-PathItemOrNull -Path (Join-Path $item.FullName '.git') -HomePath $resolved
        if ($gitMetadata) {
          throw "dev-isolated: refusing -HomeDir '$resolved' because it is inside a git workspace."
        }
      }
    }
    $parent = [System.IO.Directory]::GetParent($currentPath)
    if ($null -eq $parent) { break }
    $currentPath = $parent.FullName
  }
  return $resolved
}

function Assert-SafeIsolatedHomePaths {
  param(
    [Parameter(Mandatory = $true)][string]$HomeDir,
    [Parameter(Mandatory = $true)][string[]]$Paths
  )

  $home = Get-NormalizedPath -Path $HomeDir
  foreach ($path in $Paths) {
    $candidate = Get-NormalizedPath -Path $path
    if (-not (Test-PathWithin -Path $candidate -Root $home)) {
      throw "dev-isolated: refusing -HomeDir '$home' because path '$candidate' is outside the isolated home."
    }

    $currentPath = $candidate
    while ($true) {
      $item = Get-PathItemOrNull -Path $currentPath -HomePath $home
      if ($item) {
        if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
          throw "dev-isolated: refusing -HomeDir '$home' because path '$candidate' uses a symbolic link or junction."
        }
        if (-not $item.PSIsContainer -and
          -not [string]::Equals($currentPath, $candidate, [System.StringComparison]::OrdinalIgnoreCase)) {
          throw "dev-isolated: refusing -HomeDir '$home' because path '$candidate' passes through a file."
        }
      }

      if ([string]::Equals($currentPath, $home, [System.StringComparison]::OrdinalIgnoreCase)) { break }
      $parent = [System.IO.Directory]::GetParent($currentPath)
      if ($null -eq $parent -or -not (Test-PathWithin -Path $parent.FullName -Root $home)) {
        throw "dev-isolated: refusing -HomeDir '$home' because path '$candidate' cannot be verified inside the isolated home."
      }
      $currentPath = $parent.FullName
    }
  }
}

function New-KandevIsolatedChildEnvironment {
  param(
    [Parameter(Mandatory = $true)][string]$BinaryDirectory,
    [Parameter(Mandatory = $true)][string]$IsolatedHome,
    [System.Collections.IDictionary]$Overrides = @{}
  )

  $environment = @{}
  $inheritedNames = @(
    'SystemRoot', 'WINDIR', 'COMSPEC', 'PATHEXT', 'TEMP', 'TMP', 'SystemDrive',
    'ProgramData', 'ProgramFiles', 'ProgramFiles(x86)', 'PUBLIC', 'OS',
    'PROCESSOR_ARCHITECTURE', 'NUMBER_OF_PROCESSORS'
  )
  foreach ($name in $inheritedNames) {
    $value = [System.Environment]::GetEnvironmentVariable($name, 'Process')
    if ($null -ne $value) { $environment[$name] = $value }
  }

  $path = [System.Environment]::GetEnvironmentVariable('PATH', 'Process')
  $environment['PATH'] = if ($path) { "$BinaryDirectory;$path" } else { $BinaryDirectory }
  $environment['HOME'] = $IsolatedHome
  $environment['USERPROFILE'] = $IsolatedHome
  $environment['KANDEV_HOME_DIR'] = $IsolatedHome
  $environment['APPDATA'] = Join-Path $IsolatedHome 'AppData\Roaming'
  $environment['LOCALAPPDATA'] = Join-Path $IsolatedHome 'AppData\Local'
  foreach ($key in $Overrides.Keys) {
    if ($null -ne $Overrides[$key]) { $environment[$key] = [string]$Overrides[$key] }
  }
  return $environment
}

function Invoke-WithIsolatedEnvironment {
  param(
    [Parameter(Mandatory = $true)][System.Collections.IDictionary]$Environment,
    [Parameter(Mandatory = $true)][scriptblock]$Action
  )

  $originalEnvironment = @{}
  $currentEnvironment = [System.Environment]::GetEnvironmentVariables('Process')
  foreach ($entry in $currentEnvironment.GetEnumerator()) {
    if (-not ([string]$entry.Key).StartsWith('=')) {
      $originalEnvironment[$entry.Key] = $entry.Value
    }
  }
  try {
    foreach ($key in @($currentEnvironment.Keys)) {
      if (-not ([string]$key).StartsWith('=')) {
        [System.Environment]::SetEnvironmentVariable($key, $null, 'Process')
      }
    }
    foreach ($key in $Environment.Keys) {
      [System.Environment]::SetEnvironmentVariable($key, [string]$Environment[$key], 'Process')
    }
    & $Action
  } finally {
    $currentEnvironment = [System.Environment]::GetEnvironmentVariables('Process')
    foreach ($key in @($currentEnvironment.Keys)) {
      if (-not ([string]$key).StartsWith('=')) {
        [System.Environment]::SetEnvironmentVariable($key, $null, 'Process')
      }
    }
    foreach ($key in $originalEnvironment.Keys) {
      [System.Environment]::SetEnvironmentVariable($key, [string]$originalEnvironment[$key], 'Process')
    }
  }
}

Export-ModuleMember -Function Resolve-SafeIsolatedHome, Assert-SafeIsolatedHomePaths, New-KandevIsolatedChildEnvironment, Invoke-WithIsolatedEnvironment
