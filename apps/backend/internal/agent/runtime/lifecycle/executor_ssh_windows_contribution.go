package lifecycle

import (
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// sshWindowsContributionDestinationScript is the PowerShell form of
// scriptengine.ContributionDestinationSetupScriptAt. Origin and the branch
// upstream stay canonical; only ordinary pushes use the verified destination.
func sshWindowsContributionDestinationScript(workspacePath string, destination *models.ContributionDestination) string {
	remoteName := destination.ContributionRemoteName()
	return fmt.Sprintf(`$__kDestDir = %[1]s
$__kDestRemote = %[2]s
$__kDestUrl = %[3]s
$__kConfigured = (Invoke-KandevNative { & git -C $__kDestDir config --get ("remote." + $__kDestRemote + ".url") 2>$null } | Out-String).Trim()
if ($__kConfigured) {
  if ($__kConfigured -cne $__kDestUrl) { throw 'kandev: contribution destination identity conflict' }
} else {
  Invoke-KandevNative { & git -C $__kDestDir remote add $__kDestRemote $__kDestUrl *>$null }
  if ($LASTEXITCODE -ne 0) { throw 'kandev: contribution destination remote setup failed' }
}
$__kDestPushUrls = @(Invoke-KandevNative { & git -C $__kDestDir config --get-all ("remote." + $__kDestRemote + ".pushurl") 2>$null } |
  ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ })
if ($__kDestPushUrls.Count -gt 0) {
  foreach ($__kDestPushUrl in $__kDestPushUrls) {
    if ($__kDestPushUrl -cne $__kDestUrl) { throw 'kandev: contribution destination push identity conflict' }
  }
} else {
  Invoke-KandevNative { & git -C $__kDestDir config --add ("remote." + $__kDestRemote + ".pushurl") $__kDestUrl *>$null }
  if ($LASTEXITCODE -ne 0) { throw 'kandev: contribution destination push setup failed' }
}
$__kCurBranch = (Invoke-KandevNative { & git -C $__kDestDir branch --show-current 2>$null } | Out-String).Trim()
if ($__kCurBranch) {
  Invoke-KandevNative { & git -C $__kDestDir config ("branch." + $__kCurBranch + ".pushRemote") $__kDestRemote *>$null }
  if ($LASTEXITCODE -ne 0) { throw 'kandev: contribution destination push setup failed' }
}`,
		powerShellQuote(cleanWindowsRemotePath(workspacePath)),
		powerShellQuote(remoteName),
		powerShellQuote(destination.TargetRepository.RemoteURL),
	)
}

// sshWindowsRemoteContributionScript is the PowerShell form of
// sshRemoteContributionScript. A retained branch that already contains the
// contribution head keeps its local commits; a diverged branch is never reset,
// so the checkout moves to a new suffixed branch instead.
func sshWindowsRemoteContributionScript(taskDir, targetURL string, binding *models.RemoteContribution) string {
	remoteName, remoteBranch, remoteRef, refspec, suffix := sshRemoteContributionScriptValues(binding)
	return fmt.Sprintf(sshWindowsRemoteContributionSetup+sshWindowsRemoteContributionCheckout,
		powerShellQuote(cleanWindowsRemotePath(taskDir)),
		powerShellQuote(targetURL),
		powerShellQuote(remoteName),
		powerShellQuote(binding.SourceRepository.RemoteURL),
		powerShellQuote(binding.BaseBranch),
		powerShellQuote(remoteBranch),
		powerShellQuote(remoteRef),
		powerShellQuote(refspec),
		powerShellQuote(binding.HeadSHA),
		powerShellQuote(remoteBranch+"-kandev-"+suffix),
	)
}

const sshWindowsRemoteContributionSetup = `$__kWs = %[1]s
$__kTargetUrl = %[2]s
$__kContribRemote = %[3]s
$__kContribUrl = %[4]s
$__kBaseBranch = %[5]s
$__kContribBranch = %[6]s
$__kContribRef = %[7]s
$__kContribRefspec = %[8]s
$__kExpectedHead = (%[9]s).ToLowerInvariant()
$__kSuffixBranch = %[10]s
Invoke-KandevNative { & git -C $__kWs rev-parse --git-dir *>$null }
if ($LASTEXITCODE -eq 0) {
  $__kOrigin = (Invoke-KandevNative { & git -C $__kWs config --get remote.origin.url 2>$null } | Out-String).Trim()
  if (-not $__kOrigin) {
    Invoke-KandevNative { & git -C $__kWs remote add origin $__kTargetUrl *>$null }
    if ($LASTEXITCODE -ne 0) { throw 'kandev: target origin setup failed' }
  } elseif ($__kOrigin -cne $__kTargetUrl) { throw 'kandev: target origin identity conflict' }
} else {
  Invoke-KandevNative { & git init -q $__kWs *>$null }
  if ($LASTEXITCODE -ne 0) { throw 'kandev: workspace repository setup failed' }
  Invoke-KandevNative { & git -C $__kWs remote add origin $__kTargetUrl *>$null }
  if ($LASTEXITCODE -ne 0) { throw 'kandev: target origin setup failed' }
}
$__kGitDir = (Invoke-KandevNative { & git -C $__kWs rev-parse --absolute-git-dir 2>$null } | Out-String).Trim()
if (-not $__kGitDir) { throw 'kandev: workspace repository is unavailable' }
$__kExcludeFile = Join-Path $__kGitDir 'info\exclude'
[System.IO.Directory]::CreateDirectory([System.IO.Path]::GetDirectoryName($__kExcludeFile)) | Out-Null
if (-not (Test-Path -LiteralPath $__kExcludeFile -PathType Leaf)) { [System.IO.File]::WriteAllText($__kExcludeFile, '') }
if (@([System.IO.File]::ReadAllLines($__kExcludeFile)) -notcontains '/.kandev/') {
  [System.IO.File]::AppendAllText($__kExcludeFile, ('/.kandev/' + [Environment]::NewLine))
}
Invoke-KandevNative { & git -C $__kWs fetch --no-tags origin ("+refs/heads/" + $__kBaseBranch + ":refs/remotes/origin/" + $__kBaseBranch) *>$null }
if ($LASTEXITCODE -ne 0) { throw 'kandev: target base branch is unavailable' }
$__kRemoteUrl = (Invoke-KandevNative { & git -C $__kWs config --get ("remote." + $__kContribRemote + ".url") 2>$null } | Out-String).Trim()
if (-not $__kRemoteUrl) {
  Invoke-KandevNative { & git -C $__kWs remote add $__kContribRemote $__kContribUrl *>$null }
  if ($LASTEXITCODE -ne 0) { throw 'kandev: contribution remote setup failed' }
} elseif ($__kRemoteUrl -cne $__kContribUrl) { throw 'kandev: contribution remote identity conflict' }
$__kPushUrls = @(Invoke-KandevNative { & git -C $__kWs config --get-all ("remote." + $__kContribRemote + ".pushurl") 2>$null } |
  ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ })
if (($__kPushUrls.Count -gt 1) -or (($__kPushUrls.Count -eq 1) -and ($__kPushUrls[0] -cne $__kContribUrl))) {
  throw 'kandev: contribution push identity conflict'
}
Invoke-KandevNative { & git -C $__kWs fetch --no-tags $__kContribRemote $__kContribRefspec *>$null }
if ($LASTEXITCODE -ne 0) { throw 'kandev: contribution source branch is unavailable' }
$__kActualHead = (Invoke-KandevNative { & git -C $__kWs rev-parse --verify $__kContribRef 2>$null } | Out-String).Trim().ToLowerInvariant()
if (-not $__kActualHead -or ($__kActualHead -ne $__kExpectedHead)) { throw 'kandev: contribution source head changed' }
`

const sshWindowsRemoteContributionCheckout = `$__kCurrentBranch = (Invoke-KandevNative { & git -C $__kWs branch --show-current 2>$null } | Out-String).Trim()
$__kCurrentUpstream = (Invoke-KandevNative { & git -C $__kWs rev-parse --abbrev-ref --symbolic-full-name '@{upstream}' 2>$null } | Out-String).Trim()
$__kCheckout = $__kContribBranch
$__kKeepCurrent = $false
if ($__kCurrentBranch -and ($__kCurrentUpstream -ceq ($__kContribRemote + '/' + $__kContribBranch))) {
  Invoke-KandevNative { & git -C $__kWs merge-base --is-ancestor $__kExpectedHead HEAD *>$null }
  if ($LASTEXITCODE -eq 0) {
    $__kCheckout = $__kCurrentBranch
    $__kKeepCurrent = $true
  }
}
if (-not $__kKeepCurrent) {
  Invoke-KandevNative { & git -C $__kWs show-ref --verify --quiet ("refs/heads/" + $__kCheckout) *>$null }
  if ($LASTEXITCODE -eq 0) {
    Invoke-KandevNative { & git -C $__kWs merge-base --is-ancestor $__kExpectedHead ("refs/heads/" + $__kCheckout) *>$null }
    if ($LASTEXITCODE -ne 0) {
      $__kCheckout = $__kSuffixBranch
      $__kSuffixNumber = 0
      while ($true) {
        Invoke-KandevNative { & git -C $__kWs show-ref --verify --quiet ("refs/heads/" + $__kCheckout) *>$null }
        if ($LASTEXITCODE -ne 0) { break }
        $__kSuffixNumber++
        $__kCheckout = $__kSuffixBranch + '-' + $__kSuffixNumber
      }
    }
  }
}
if ($__kCurrentBranch -cne $__kCheckout) {
  Invoke-KandevNative { & git -C $__kWs show-ref --verify --quiet ("refs/heads/" + $__kCheckout) *>$null }
  if ($LASTEXITCODE -eq 0) {
    Invoke-KandevNative { & git -C $__kWs checkout $__kCheckout *>$null }
  } else {
    Invoke-KandevNative { & git -C $__kWs checkout -b $__kCheckout $__kContribRef *>$null }
  }
  if ($LASTEXITCODE -ne 0) { throw 'kandev: contribution checkout failed' }
}
Invoke-KandevNative { & git -C $__kWs branch ("--set-upstream-to=" + $__kContribRemote + "/" + $__kContribBranch) $__kCheckout *>$null }
if ($LASTEXITCODE -ne 0) { throw 'kandev: contribution upstream setup failed' }`
