#Requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Assert-Launch {
  param([bool]$Condition, [string]$Message)
  if (-not $Condition) { throw $Message }
}

function Wait-Until {
  param([scriptblock]$Condition, [string]$Message, [int]$Seconds = 20)
  $deadline = [datetime]::UtcNow.AddSeconds($Seconds)
  do {
    if (& $Condition) { return }
    Start-Sleep -Milliseconds 100
  } while ([datetime]::UtcNow -lt $deadline)
  throw $Message
}

function Get-FreeTestPort {
  $socket = New-Object Net.Sockets.TcpListener([Net.IPAddress]::Loopback, 0)
  $socket.Start()
  try { return $socket.LocalEndpoint.Port } finally { $socket.Stop() }
}

function Quote-PowerShellLiteral {
  param([string]$Value)
  return "'" + $Value.Replace("'", "''") + "'"
}

function Start-TestLauncher {
  param([string]$Extra = '', [int]$Timeout = 4, [switch]$Build, [int]$BuildExit = 0, [switch]$StaleFirst)
  $command = '$ErrorActionPreference = ''Stop''; $ProgressPreference = ''SilentlyContinue''; '
  if ($StaleFirst) {
    # Keep the actual OS rows, ordered so the unrelated listener is first.
    $command += @'
function Get-NetTCPConnection {
  [CmdletBinding()]
  param([string]$State, [int]$LocalPort)
  NetTCPIP\Get-NetTCPConnection @PSBoundParameters | Sort-Object {
    (Get-CimInstance Win32_Process -Filter "ProcessId = $($_.OwningProcess)").CreationDate
  }
};
'@
  }
  $command += '& ' + (Quote-PowerShellLiteral $script:launcher) +
    $(if ($Build) { '' } else { ' -NoBuild' }) + ' -BackendPort ' + $script:backendPort + ' -WebPort ' + $script:webPort +
    ' -HomeDir ' + (Quote-PowerShellLiteral $script:homeDir) + ' -Timeout ' + $Timeout + ' ' + $Extra
  $encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($command))
  $start = New-Object Diagnostics.ProcessStartInfo
  $start.FileName = Join-Path $PSHOME 'powershell.exe'
  $start.Arguments = '-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand ' + $encoded
  $start.UseShellExecute = $false
  $start.CreateNoWindow = $true
  $start.RedirectStandardOutput = $true
  $start.RedirectStandardError = $true
  $start.EnvironmentVariables['PATH'] = $script:binDir + ';' + $env:PATH
  $start.EnvironmentVariables['KANDEV_DATABASE_DRIVER'] = 'must-not-inherit'
  $start.EnvironmentVariables['DOCKER_HOST'] = 'must-not-inherit'
  $start.EnvironmentVariables['VITE_SECRET_FOR_TEST'] = 'must-not-inherit'
  $start.EnvironmentVariables['KANDEV_FIXTURE_BUILD_EXIT'] = [string]$BuildExit
  $process = [Diagnostics.Process]::Start($start)
  return [pscustomobject]@{
    Process = $process
    Output = $process.StandardOutput.ReadToEndAsync()
    Error = $process.StandardError.ReadToEndAsync()
  }
}

function Complete-TestLauncher {
  param($Run)
  Assert-Launch ($Run.Process.WaitForExit(45000)) 'Launcher exceeded its bounded startup budget.'
  Assert-Launch ($Run.Output.Wait(5000) -and $Run.Error.Wait(5000)) `
    'Caller pipes must reach EOF while the detached listeners remain alive.'
  $script:lastOutput = $Run.Output.Result + $Run.Error.Result
  Write-Output $script:lastOutput
  return $Run.Process.ExitCode
}

function Read-FixturePid {
  param([string]$Role)
  return [int](Get-Content -LiteralPath (Join-Path $script:homeDir ($Role + '.pid')) -Raw)
}

function Start-CompetingListener {
  param([string]$Role, [int]$Port, [string]$HostAddress = '127.0.0.1')
  $competitorHome = Join-Path $script:homeDir ('competitor-' + $Role)
  New-Item -ItemType Directory -Path $competitorHome | Out-Null
  Set-Content -LiteralPath (Join-Path $competitorHome 'release') -Value 'go'
  $start = New-Object Diagnostics.ProcessStartInfo
  $start.FileName = Join-Path $script:binDir $(if ($Role -eq 'web') { 'node.exe' } else { 'kandev.exe' })
  $start.Arguments = if ($Role -eq 'web') {
    'node_modules/vite/bin/vite.js --host ' + $HostAddress + ' --port ' + $Port + ' --strictPort'
  } else { '__backend' }
  $start.UseShellExecute = $false
  $start.CreateNoWindow = $true
  $start.EnvironmentVariables['KANDEV_HOME_DIR'] = $competitorHome
  $start.EnvironmentVariables['KANDEV_SERVER_PORT'] = [string]$Port
  $start.EnvironmentVariables['KANDEV_SERVER_HOST'] = $HostAddress
  $competitor = [Diagnostics.Process]::Start($start)
  try {
    Wait-Until { @(Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue | ForEach-Object { $_.OwningProcess }) -contains $competitor.Id } `
      'Concurrent compatible listener failed to bind.'
    return $competitor
  } catch {
    if (-not $competitor.HasExited) { $competitor.Kill(); $competitor.WaitForExit() }
    $competitor.Dispose()
    throw
  }
}

function Assert-FixtureGone {
  foreach ($file in @(Get-ChildItem -LiteralPath $script:homeDir -Filter '*.pid' -ErrorAction SilentlyContinue)) {
    $target = [int](Get-Content -LiteralPath $file.FullName -Raw)
    Wait-Until { -not (Get-Process -Id $target -ErrorAction SilentlyContinue) } `
      "Owned fixture $($file.Name) PID $target survived failed launch cleanup." -Seconds 8
  }
}

function Invoke-LaunchCase {
  param([string]$Name, [scriptblock]$Body)
  $script:homeDir = Join-Path $script:testRoot ([guid]::NewGuid().ToString('N'))
  New-Item -ItemType Directory -Path $script:homeDir | Out-Null
  $script:backendPort = Get-FreeTestPort
  do { $script:webPort = Get-FreeTestPort } while ($script:webPort -eq $script:backendPort)
  $script:pidfile = Join-Path $env:TEMP ("kandev-dev-isolated-$($script:backendPort).pid")
  $script:lastOutput = ''
  $script:injectionMarker = $null
  Set-Content -LiteralPath (Join-Path $script:binDir 'backend.mode') -Value 'healthy' -Encoding ASCII
  Set-Content -LiteralPath (Join-Path $script:binDir 'web.mode') -Value 'healthy' -Encoding ASCII
  try {
    & $Body
    Write-Output "PASS: $Name"
  } catch {
    $script:failures += $Name + ': ' + $_.Exception.Message
    Write-Output "FAIL: $Name : $($_.Exception.Message)"
    Write-Output $script:lastOutput
  } finally {
    # Only processes with fixture identities in this case's owned directory.
    foreach ($file in @(Get-ChildItem -LiteralPath $script:homeDir -Filter '*.pid' -ErrorAction SilentlyContinue)) {
      $target = [int](Get-Content -LiteralPath $file.FullName -Raw)
      $info = Get-CimInstance Win32_Process -Filter "ProcessId = $target" -ErrorAction SilentlyContinue
      if ($info -and $info.ExecutablePath -and $info.ExecutablePath.StartsWith($script:testRoot)) {
        $previousErrorAction = $ErrorActionPreference
        try {
          $ErrorActionPreference = 'Continue'
          & "$env:SystemRoot\System32\taskkill.exe" /PID $target /T /F 2>&1 | Out-Null
        } finally { $ErrorActionPreference = $previousErrorAction }
      }
    }
    foreach ($suffix in @('.pid', '.backend.started', '.home', '.web.pid', '.web.started')) {
      $metadata = $script:pidfile -replace '\.pid$', $suffix
      Remove-Item -LiteralPath $metadata -Force -Recurse -ErrorAction SilentlyContinue
    }
    if ($script:injectionMarker) { Remove-Item -LiteralPath $script:injectionMarker -Force -ErrorAction SilentlyContinue }
  }
}

$scriptsDir = Split-Path -Parent $PSScriptRoot
$script:testRoot = Join-Path $env:TEMP ('isolated launch %TEMP% ! & ^ (x) ' + [guid]::NewGuid().ToString('N'))
$script:binDir = Join-Path $script:testRoot 'repo\apps\backend\bin'
$webDir = Join-Path $script:testRoot 'repo\apps\web\node_modules\vite\bin'
$copyScripts = Join-Path $script:testRoot 'repo\scripts'
$script:launcher = Join-Path $copyScripts 'dev-isolated.ps1'
$script:failures = @()
New-Item -ItemType Directory -Path $script:binDir, $webDir, $copyScripts, (Join-Path $script:testRoot 'repo\apps\node_modules') -Force | Out-Null
try {
  Get-ChildItem -LiteralPath $scriptsDir -File | Where-Object { $_.Extension -in @('.ps1', '.psm1', '.cs') } |
    Copy-Item -Destination $copyScripts
  New-Item -ItemType File -Path (Join-Path $webDir 'vite.js') | Out-Null
  $fixtureExe = Join-Path $script:binDir 'kandev.exe'
  $compiledFixture = Join-Path $env:TEMP ('isolated-fixture-' + [guid]::NewGuid().ToString('N') + '.exe')
  try {
    Add-Type -TypeDefinition (Get-Content -LiteralPath (Join-Path $PSScriptRoot 'isolated-launch-fixture.cs') -Raw) `
      -OutputAssembly $compiledFixture -OutputType ConsoleApplication
    Copy-Item -LiteralPath $compiledFixture -Destination $fixtureExe
  } finally {
    Remove-Item -LiteralPath $compiledFixture -Force -ErrorAction SilentlyContinue
  }
  foreach ($name in @('node.exe', 'agentctl.exe', 'mock-agent.exe', 'make.exe', 'go.exe')) {
    Copy-Item -LiteralPath $fixtureExe -Destination (Join-Path $script:binDir $name)
  }
  $winjobSource = Join-Path (Split-Path -Parent $scriptsDir) 'apps\backend\bin\winjob.exe'
  Copy-Item -LiteralPath $winjobSource -Destination (Join-Path $script:binDir 'winjob.exe')
  Set-Content -LiteralPath (Join-Path $script:testRoot 'repo\apps\backend\newer.go') -Value '// fixture build input' -Encoding ASCII

  Invoke-LaunchCase 'real backend and Vite listeners outlive caller pipe EOF; literal paths and isolated environment' {
    $run = Start-TestLauncher -Extra '-Web'
    $exitCode = @(Complete-TestLauncher $run)[-1]
    Assert-Launch ($exitCode -eq 0) 'Healthy backend and Vite launch must succeed.'
    foreach ($role in @('backend', 'web')) {
      $target = Read-FixturePid $role
      $port = if ($role -eq 'backend') { $script:backendPort } else { $script:webPort }
      $record = if ($role -eq 'backend') { $script:pidfile } else { $script:pidfile -replace '\.pid$', '.web.pid' }
      Assert-Launch ([int](Get-Content -LiteralPath $record -Raw) -eq $target) "$role metadata must record the actual fixture listener."
      Assert-Launch (@(Get-NetTCPConnection -State Listen -LocalPort $port).OwningProcess -contains $target) "$role must actually listen."
      Assert-Launch ((Get-Content -LiteralPath (Join-Path $script:homeDir ($role + '.environment')) -Raw) -eq '||') `
        "$role inherited a forbidden environment variable."
    }
    & (Join-Path $copyScripts 'kandev-kill.ps1') -Pidfile $script:pidfile -Yes
    Assert-FixtureGone
  }

  foreach ($competitorHealth in @('healthy', 'unhealthy')) {
  Invoke-LaunchCase "a concurrent unrelated backend ($competitorHealth) is never adopted or stopped" {
    Set-Content -LiteralPath (Join-Path $script:binDir 'backend.mode') -Value ('delay ' + $competitorHealth) -Encoding ASCII
    $run = Start-TestLauncher
    Wait-Until { Test-Path -LiteralPath (Join-Path $script:homeDir 'backend.pid') } 'Owned backend never reached the launch barrier.'
    $competitor = Start-CompetingListener -Role backend -Port $script:backendPort
    try {
      Set-Content -LiteralPath (Join-Path $script:homeDir 'release') -Value 'go'
      $exitCode = @(Complete-TestLauncher $run)[-1]
      Assert-Launch ($exitCode -ne 0) 'Launcher adopted the concurrent unrelated backend listener.'
      Assert-Launch (-not $competitor.HasExited) 'Failure cleanup stopped the unrelated listener.'
      Assert-Launch (-not (Test-Path -LiteralPath $script:pidfile)) 'Failed launch left adopted metadata.'
      Assert-FixtureGone
    } finally {
      if (-not $competitor.HasExited) { $competitor.Kill(); $competitor.WaitForExit() }
      $competitor.Dispose()
    }
  }
  }

  Invoke-LaunchCase 'a stale first listener row does not hide the owned backend' {
    $competitor = Start-CompetingListener -Role backend -Port $script:backendPort -HostAddress '127.0.0.2'
    try {
      $run = Start-TestLauncher -StaleFirst
      $exitCode = @(Complete-TestLauncher $run)[-1]
      Assert-Launch ($exitCode -eq 0) 'The owned listener must be selected from mixed listener rows.'
      Assert-Launch (@(Get-NetTCPConnection -State Listen -LocalPort $script:backendPort).Count -ge 2) 'Mixed-listener positive control needs two real rows.'
      Assert-Launch ([int](Get-Content -LiteralPath $script:pidfile -Raw) -eq (Read-FixturePid backend)) 'Metadata selected the stale unrelated listener.'
      & (Join-Path $copyScripts 'kandev-kill.ps1') -Pidfile $script:pidfile -Yes
      Assert-Launch (-not $competitor.HasExited) 'Normal teardown stopped the unrelated stale listener.'
      Assert-FixtureGone
    } finally {
      if (-not $competitor.HasExited) { $competitor.Kill(); $competitor.WaitForExit() }
      $competitor.Dispose()
    }
  }

  Invoke-LaunchCase 'a concurrent unrelated Vite listener is never adopted or stopped' {
    Set-Content -LiteralPath (Join-Path $script:binDir 'web.mode') -Value 'delay' -Encoding ASCII
    $run = Start-TestLauncher -Extra '-Web'
    Wait-Until { Test-Path -LiteralPath (Join-Path $script:homeDir 'web.pid') } 'Owned Vite never reached its launch barrier.'
    $competitor = Start-CompetingListener -Role web -Port $script:webPort
    try {
      Set-Content -LiteralPath (Join-Path $script:homeDir 'release') -Value 'go'
      $exitCode = @(Complete-TestLauncher $run)[-1]
      Assert-Launch ($exitCode -ne 0) 'Launcher adopted a concurrent unrelated Vite listener.'
      Assert-Launch (-not $competitor.HasExited) 'Failure cleanup stopped the unrelated Vite listener.'
      Assert-FixtureGone
    } finally {
      if (-not $competitor.HasExited) { $competitor.Kill(); $competitor.WaitForExit() }
      $competitor.Dispose()
    }
  }

  Invoke-LaunchCase 'health timeout cleans the full owned descendant tree' {
    Set-Content -LiteralPath (Join-Path $script:binDir 'backend.mode') -Value 'unhealthy tree' -Encoding ASCII
    $run = Start-TestLauncher
    $exitCode = @(Complete-TestLauncher $run)[-1]
    Assert-Launch ($exitCode -ne 0) 'Unhealthy backend must fail startup.'
    Assert-Launch (Test-Path -LiteralPath (Join-Path $script:homeDir 'descendant-grandchild.pid')) 'Grandchild positive control did not start.'
    Assert-FixtureGone
  }

  Invoke-LaunchCase 'discovery timeout cleans the full owned descendant tree' {
    Set-Content -LiteralPath (Join-Path $script:binDir 'backend.mode') -Value 'no-listener tree' -Encoding ASCII
    $run = Start-TestLauncher
    $exitCode = @(Complete-TestLauncher $run)[-1]
    Assert-Launch ($exitCode -ne 0) 'Backend without a listener must fail startup.'
    Assert-Launch (Test-Path -LiteralPath (Join-Path $script:homeDir 'descendant-grandchild.pid')) 'Grandchild positive control did not start.'
    Assert-FixtureGone
  }

  Invoke-LaunchCase 'backend metadata write failure cleans launched processes' {
    New-Item -ItemType Directory -Path ($script:pidfile -replace '\.pid$', '.backend.started') | Out-Null
    $run = Start-TestLauncher
    $exitCode = @(Complete-TestLauncher $run)[-1]
    Assert-Launch ($exitCode -ne 0) 'Injected metadata write failure must fail startup.'
    Assert-Launch (Test-Path -LiteralPath (Join-Path $script:homeDir 'backend.pid')) 'Backend positive control did not start.'
    Assert-FixtureGone
  }

  Invoke-LaunchCase 'web metadata write failure cleans both owned trees' {
    New-Item -ItemType Directory -Path ($script:pidfile -replace '\.pid$', '.web.started') | Out-Null
    Set-Content -LiteralPath (Join-Path $script:binDir 'web.mode') -Value 'healthy tree' -Encoding ASCII
    $run = Start-TestLauncher -Extra '-Web'
    $exitCode = @(Complete-TestLauncher $run)[-1]
    Assert-Launch ($exitCode -ne 0) 'Injected web metadata write failure must fail startup.'
    Assert-Launch (Test-Path -LiteralPath (Join-Path $script:homeDir 'web.pid')) 'Vite positive control did not start.'
    Assert-Launch (Test-Path -LiteralPath (Join-Path $script:homeDir 'descendant-grandchild.pid')) 'Web grandchild positive control did not start.'
    Assert-FixtureGone
  }

  foreach ($hostAddress in @('localhost', '::1', '[::1]', '0.0.0.0')) {
    Invoke-LaunchCase "legitimate web bind host $hostAddress" {
      $run = Start-TestLauncher -Extra ('-Web -WebHost ' + (Quote-PowerShellLiteral $hostAddress))
      $exitCode = @(Complete-TestLauncher $run)[-1]
      Assert-Launch ($exitCode -eq 0) 'A legitimate host must launch.'
      Assert-Launch ($script:lastOutput -match 'Vite target') 'The selected host must actually become ready.'
      Assert-Launch (Test-Path -LiteralPath (Join-Path $script:homeDir 'web.pid')) 'Vite positive control did not start.'
    }
  }

  foreach ($role in @('backend', 'web')) {
    Invoke-LaunchCase "$role start-time mismatch refuses teardown; matching identity stops it" {
      $run = Start-TestLauncher -Extra '-Web'
      $exitCode = @(Complete-TestLauncher $run)[-1]
      Assert-Launch ($exitCode -eq 0) 'Identity guard positive control must launch.'
      $startedFile = $script:pidfile -replace '\.pid$', $(if ($role -eq 'web') { '.web.started' } else { '.backend.started' })
      $recorded = Get-Content -LiteralPath $startedFile -Raw
      Set-Content -LiteralPath $startedFile -Value '0' -Encoding ASCII
      & (Join-Path $copyScripts 'kandev-kill.ps1') -Pidfile $script:pidfile -Yes
      Assert-Launch ($LASTEXITCODE -eq 4) 'Mismatched start time must refuse teardown.'
      Assert-Launch ($null -ne (Get-Process -Id (Read-FixturePid $role) -ErrorAction SilentlyContinue)) 'Identity mismatch stopped the protected listener.'
      Set-Content -LiteralPath $startedFile -Value $recorded -Encoding ASCII
      & (Join-Path $copyScripts 'kandev-kill.ps1') -Pidfile $script:pidfile -Yes
      Assert-FixtureGone
    }
  }

  Invoke-LaunchCase 'WebHost shell metacharacters are refused before launching' {
    $marker = Join-Path $env:TEMP ('isolated-host-injection-' + [guid]::NewGuid().ToString('N') + '.txt')
    $script:injectionMarker = $marker
    $hostArgument = '127.0.0.1 & echo INJECTED>"' + $marker + '" & rem '
    $run = Start-TestLauncher -Extra ('-Web -WebHost ' + (Quote-PowerShellLiteral $hostArgument))
    $exitCode = @(Complete-TestLauncher $run)[-1]
    Assert-Launch ($exitCode -ne 0) 'Shell-source host argument must be rejected.'
    Assert-Launch (-not (Test-Path -LiteralPath $marker)) 'WebHost was evaluated as shell source.'
    Assert-Launch ($script:lastOutput -match 'WebHost') 'Host rejection must identify the invalid parameter.'
    Assert-Launch (-not (Test-Path -LiteralPath (Join-Path $script:homeDir 'backend.pid'))) 'Invalid host must fail before backend launch.'
  }

  Invoke-LaunchCase 'Command Processor AutoRun cannot restore host settings in isolated children' {
    $key = 'HKCU:\Software\Microsoft\Command Processor'
    $keyExisted = Test-Path -LiteralPath $key
    if (-not $keyExisted) { New-Item -Path $key -Force | Out-Null }
    $registry = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Software\Microsoft\Command Processor', $true)
    $hadValue = $registry.GetValueNames() -contains 'AutoRun'
    $previous = if ($hadValue) { $registry.GetValue('AutoRun', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames) } else { $null }
    $previousKind = if ($hadValue) { $registry.GetValueKind('AutoRun') } else { [Microsoft.Win32.RegistryValueKind]::String }
    $marker = Join-Path $env:TEMP ('isolated-autorun-' + [guid]::NewGuid().ToString('N') + '.txt')
    $script:injectionMarker = $marker
    try {
      $registry.SetValue('AutoRun', 'set KANDEV_DATABASE_DRIVER=autorun-host&set DOCKER_HOST=autorun-host&echo ran>"' + $marker + '"', [Microsoft.Win32.RegistryValueKind]::String)
      # Prove this exact registry payload is active before testing the launcher.
      & "$env:SystemRoot\System32\cmd.exe" /c exit 0
      Assert-Launch (Test-Path -LiteralPath $marker) 'AutoRun positive control was not executed.'
      Remove-Item -LiteralPath $marker -Force
      $run = Start-TestLauncher -Extra '-Web'
      $exitCode = @(Complete-TestLauncher $run)[-1]
      Assert-Launch ($exitCode -eq 0) 'Both isolated children must launch despite an operator AutoRun.'
      Assert-Launch (-not (Test-Path -LiteralPath $marker)) 'Isolated launch executed Command Processor AutoRun.'
      foreach ($role in @('backend', 'web')) {
        Assert-Launch ((Get-Content -LiteralPath (Join-Path $script:homeDir ($role + '.environment')) -Raw) -eq '||') `
          "$role received settings restored by AutoRun."
      }
    } finally {
      if ($hadValue) { $registry.SetValue('AutoRun', $previous, $previousKind) } else { $registry.DeleteValue('AutoRun', $false) }
      $registry.Dispose()
      if (-not $keyExisted) { Remove-Item -LiteralPath $key -Force }
    }
  }

  Invoke-LaunchCase 'square brackets in repository paths remain literal' {
    $savedLauncher = $script:launcher
    $savedBin = $script:binDir
    $literalRepo = Join-Path $script:testRoot 'repo [literal]'
    Copy-Item -LiteralPath (Join-Path $script:testRoot 'repo') -Destination $literalRepo -Recurse
    try {
      $script:launcher = Join-Path $literalRepo 'scripts\dev-isolated.ps1'
      $script:binDir = Join-Path $literalRepo 'apps\backend\bin'
      $run = Start-TestLauncher -Extra '-Web'
      $exitCode = @(Complete-TestLauncher $run)[-1]
      Assert-Launch ($exitCode -eq 0) 'Literal repository path must launch both listeners.'
      Assert-Launch (Test-Path -LiteralPath (Join-Path $script:homeDir 'web.pid')) 'Vite positive control did not start.'
    } finally {
      $script:launcher = $savedLauncher
      $script:binDir = $savedBin
    }
  }

  Invoke-LaunchCase 'successful build stderr remains diagnostic' {
    $run = Start-TestLauncher -Build
    $exitCode = @(Complete-TestLauncher $run)[-1]
    Assert-Launch ($exitCode -eq 0) 'Successful build with diagnostic stderr must launch.'
    Assert-Launch ($script:lastOutput -match 'fixture build diagnostic') 'Build stderr positive control was not observed.'
    Assert-Launch (Test-Path -LiteralPath (Join-Path $script:homeDir 'backend.pid')) 'Backend must launch after successful build.'
  }

  Invoke-LaunchCase 'failed native build never launches a backend' {
    $run = Start-TestLauncher -Build -BuildExit 7
    $exitCode = @(Complete-TestLauncher $run)[-1]
    Assert-Launch ($exitCode -ne 0) 'Failed native build must fail startup.'
    Assert-Launch ($script:lastOutput -match 'build failed \(exit 7\)') 'Build failure must preserve the actual native exit.'
    Assert-Launch (-not (Test-Path -LiteralPath (Join-Path $script:homeDir 'backend.pid'))) 'Failed build started a backend.'
  }
} finally {
  Remove-Item -LiteralPath $script:testRoot -Recurse -Force -ErrorAction SilentlyContinue
}
if ($script:failures.Count) { throw ($script:failures -join [Environment]::NewLine) }
Write-Output 'Windows isolated launch regressions passed.'
