package lifecycle

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/scriptengine"
)

const windowsBrokerReachabilityScript = `if (-not $env:KANDEV_GITHUB_CREDENTIAL_BROKER_URL) {
  [Console]::Error.WriteLine('KANDEV_GITHUB_CREDENTIAL_BROKER_URL is missing')
  exit 69
}
try {
  $req = [System.Net.HttpWebRequest]::Create($env:KANDEV_GITHUB_CREDENTIAL_BROKER_URL)
  $req.Method = 'GET'
  $req.Timeout = 10000
  $req.ReadWriteTimeout = 10000
  $resp = [System.Net.HttpWebResponse]$req.GetResponse()
  $status = [int]$resp.StatusCode
  $resp.Close()
} catch [System.Net.WebException] {
  if ($_.Exception.Response) {
    $status = [int]([System.Net.HttpWebResponse]$_.Exception.Response).StatusCode
    $_.Exception.Response.Close()
  } else {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 69
  }
}
if ($status -ne 204) {
  [Console]::Error.WriteLine("Kandev Git credential broker readiness returned HTTP $status")
  exit 69
}
`

const defaultWindowsSSHPrepareScript = `# Prepare an SSH task workspace on a Windows remote host.
$ErrorActionPreference = 'Stop'

$workspace = {{workspace.path}}
$repositoryUrl = {{repository.clone_url}}
$repositoryBranch = {{repository.branch}}

{{github.auth_setup}}
{{git.identity_setup}}

[System.IO.Directory]::CreateDirectory($workspace) | Out-Null

if (-not [string]::IsNullOrWhiteSpace($repositoryUrl)) {
  Invoke-KandevNative { & git -C $workspace rev-parse --git-dir *>$null }
  if ($LASTEXITCODE -eq 0) {
    $configuredUrl = (Invoke-KandevNative { & git -C $workspace config --get remote.origin.url 2>$null } | Out-String).Trim()
    if ([string]::IsNullOrWhiteSpace($configuredUrl)) {
      & git -C $workspace remote add origin $repositoryUrl
      if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    } elseif ($configuredUrl -ne $repositoryUrl) {
      [Console]::Error.WriteLine('kandev: target origin identity conflict')
      exit 1
    }
  } else {
    & git init -q $workspace
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    & git -C $workspace remote add origin $repositoryUrl
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  }

  $gitDir = (& git -C $workspace rev-parse --absolute-git-dir | Out-String).Trim()
  $excludeFile = Join-Path $gitDir 'info\exclude'
  [System.IO.Directory]::CreateDirectory([System.IO.Path]::GetDirectoryName($excludeFile)) | Out-Null
  if (-not (Test-Path -LiteralPath $excludeFile -PathType Leaf)) {
    [System.IO.File]::WriteAllText($excludeFile, '')
  }
  $excludeLines = [System.IO.File]::ReadAllLines($excludeFile)
  if ($excludeLines -notcontains '/.kandev/') {
    [System.IO.File]::AppendAllText($excludeFile, ('/.kandev/' + [Environment]::NewLine))
  }

  if ([string]::IsNullOrWhiteSpace($repositoryBranch)) {
    [Console]::Error.WriteLine('kandev: target repository has no base branch')
    exit 1
  }
  Invoke-KandevNative { & git -C $workspace fetch --no-tags origin ("+refs/heads/" + $repositoryBranch + ":refs/remotes/origin/" + $repositoryBranch) *>$null }
  if ($LASTEXITCODE -ne 0) {
    [Console]::Error.WriteLine('kandev: target base branch is unavailable')
    exit 1
  }
  Invoke-KandevNative { & git -C $workspace rev-parse --verify HEAD *>$null }
  if ($LASTEXITCODE -ne 0) {
    & git -C $workspace checkout -b $repositoryBranch ("origin/" + $repositoryBranch)
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  }
}

Set-Location -LiteralPath $workspace
{{repository.setup_script}}
`

const windowsBranchCheckoutPostlude = `
# ---- kandev-managed: ensure session feature branch is checked out ----
try {
  $__kWorktreeBranch = {{worktree.branch}}
  $__kRepositoryBranch = {{repository.branch}}
  $__kWorkspace = {{workspace.path}}
  if ((Test-Path -LiteralPath (Join-Path $__kWorkspace '.git')) -and
      (-not [string]::IsNullOrWhiteSpace($__kWorktreeBranch)) -and
      ($__kWorktreeBranch -ne $__kRepositoryBranch)) {
    Push-Location -LiteralPath $__kWorkspace
    try {
      $__kShallow = (Invoke-KandevNative { & git rev-parse --is-shallow-repository 2>$null } | Out-String).Trim()
      if ($__kShallow -eq 'true') {
        Invoke-KandevNative { & git fetch --unshallow --no-tags origin *>$null }
      }
      Invoke-KandevNative { & git rev-parse --verify $__kWorktreeBranch *>$null }
      if ($LASTEXITCODE -eq 0) {
        Invoke-KandevNative { & git checkout $__kWorktreeBranch *>$null }
      } else {
        Invoke-KandevNative { & git fetch --no-tags origin ("+refs/heads/" + $__kWorktreeBranch + ":refs/remotes/origin/" + $__kWorktreeBranch) *>$null }
        if ($LASTEXITCODE -eq 0) {
          Invoke-KandevNative { & git checkout -b $__kWorktreeBranch ("origin/" + $__kWorktreeBranch) *>$null }
        } else {
          Invoke-KandevNative { & git checkout -b $__kWorktreeBranch *>$null }
        }
      }
    } finally {
      Pop-Location
    }
  }
} catch {}
`

func isBuiltinPOSIXSSHPrepareScript(script string) bool {
	trimmed := strings.TrimSpace(script)
	builtin := strings.TrimSpace(defaultSSHPrepareScript)
	return trimmed == builtin || strings.TrimSpace(repairLegacyFetchRefspec(trimmed)) == builtin
}

func (r *SSHExecutor) resolveWindowsPrepareScript(req *ExecutorCreateRequest, workspacePath string) (string, error) {
	script := strings.TrimSpace(getMetadataString(req.Metadata, MetadataKeySetupScript))
	if script == "" || isBuiltinPOSIXSSHPrepareScript(script) {
		script = defaultWindowsSSHPrepareScript
	}
	script = withWindowsBranchCheckout(req, script)
	if binding, ok := req.RemoteContributions[""]; ok {
		targetURL := getMetadataString(req.Metadata, "repository_clone_url")
		if targetURL == "" {
			return "", fmt.Errorf("ssh: remote contribution target has no clone URL")
		}
		script += "\n" + sshWindowsRemoteContributionScript(workspacePath, targetURL, &binding)
	}
	if destination, ok := req.ContributionDestinations[""]; ok {
		if err := destination.Validate(); err != nil {
			return "", err
		}
		script += "\n" + sshWindowsContributionDestinationScript(workspacePath, &destination)
	}
	return r.resolveWindowsSSHScript(req, workspacePath, script)
}

func (r *SSHExecutor) resolveWindowsSSHScript(req *ExecutorCreateRequest, workspacePath, script string) (string, error) {
	cleanWorkspace := cleanWindowsRemotePath(workspacePath)
	return resolveSSHScriptPlaceholders(script,
		windowsWorkspaceProvider(cleanWorkspace),
		windowsGitIdentityProvider(req.Metadata),
		windowsGitHubAuthProvider(req.Env),
		scriptengine.AgentInstallProvider(r.collectAgentInstallScripts(req)),
		windowsWorktreeProvider(
			cleanWorkspace,
			getMetadataString(req.Metadata, MetadataKeyWorktreeID),
			getMetadataString(req.Metadata, MetadataKeyWorktreeBranch),
			getMetadataString(req.Metadata, MetadataKeyBaseBranch),
		),
		windowsRepositoryProvider(req.Metadata),
	)
}

func windowsWorkspaceProvider(workspacePath string) scriptengine.PlaceholderProvider {
	return func() map[string]string {
		return map[string]string{"workspace.path": powerShellQuote(workspacePath)}
	}
}

func windowsWorktreeProvider(workspacePath, id, branch, baseBranch string) scriptengine.PlaceholderProvider {
	return func() map[string]string {
		return map[string]string{
			"worktree.base_path":   powerShellQuote(workspacePath),
			"worktree.path":        powerShellQuote(workspacePath),
			"worktree.id":          id,
			"worktree.branch":      powerShellQuote(branch),
			"worktree.base_branch": powerShellQuote(baseBranch),
		}
	}
}

func windowsRepositoryProvider(metadata map[string]interface{}) scriptengine.PlaceholderProvider {
	return func() map[string]string {
		vars := make(map[string]string)
		repoPath := getMetadataString(metadata, "repository_path")
		if repoPath != "" {
			vars["repository.path"] = powerShellQuote(repoPath)
			slashed := strings.ReplaceAll(strings.TrimRight(repoPath, `\/`), `\`, "/")
			name := slashed
			if idx := strings.LastIndexByte(slashed, '/'); idx >= 0 {
				name = slashed[idx+1:]
			}
			vars["repository.name"] = powerShellQuote(name)
		}
		branch := getMetadataString(metadata, "base_branch")
		if branch == "" {
			branch = getMetadataString(metadata, "repository_branch")
		}
		vars["repository.branch"] = powerShellQuote(scriptengine.RepositoryBranchName(branch))
		vars["repository.setup_script"] = getMetadataString(metadata, "repository_setup_script")
		if cloneURL := getMetadataString(metadata, "repository_clone_url"); cloneURL != "" {
			vars["repository.clone_url"] = powerShellQuote(cloneURL)
		} else if repoPath != "" {
			if remoteURL, err := getGitRemoteURL(repoPath); err == nil && remoteURL != "" {
				vars["repository.ssh_url"] = powerShellQuote(remoteURL)
				vars["repository.clone_url"] = powerShellQuote(remoteURL)
			}
		}
		return vars
	}
}

func windowsGitIdentityProvider(metadata map[string]interface{}) scriptengine.PlaceholderProvider {
	return func() map[string]string {
		name := getMetadataString(metadata, "git_user_name")
		email := getMetadataString(metadata, "git_user_email")
		vars := map[string]string{
			"git.user_name":      name,
			"git.user_email":     email,
			"git.identity_setup": "",
		}
		if name == "" || email == "" {
			return vars
		}
		vars["git.identity_setup"] = fmt.Sprintf(
			"& git config --global user.name %s\n& git config --global user.email %s",
			powerShellQuote(name), powerShellQuote(email),
		)
		return vars
	}
}

func windowsGitHubAuthProvider(env map[string]string) scriptengine.PlaceholderProvider {
	return func() map[string]string {
		token := ""
		if env != nil {
			token = env[envKeyGHToken]
			if token == "" {
				token = env[envKeyGitHubToken]
			}
		}
		ghSnippet := `if (Get-Command gh -ErrorAction SilentlyContinue) {
  Invoke-KandevNative { & gh config set git_protocol https --host github.com *>$null }
  Invoke-KandevNative { & gh auth setup-git *>$null }
}`
		if token == "" {
			return map[string]string{"github.auth_setup": ghSnippet}
		}
		credHelper := `!sh -c "echo username=x-access-token; echo password=${GH_TOKEN:-${GITHUB_TOKEN}}"`
		setup := fmt.Sprintf("& git config --global --replace-all credential.https://github.com.helper %s\n%s",
			powerShellQuote(credHelper), ghSnippet)
		return map[string]string{"github.auth_setup": setup}
	}
}

func sshWindowsOrphanInventoryScript(resolvedRoot string) string {
	tasksRoot := joinWindowsRemotePath(resolvedRoot, "tasks")
	script := fmt.Sprintf(`$root = %s
$procs = Get-CimInstance -ClassName Win32_Process -Filter "Name = 'agentctl.exe'" -ErrorAction SilentlyContinue
foreach ($p in $procs) {
  $cmd = [string]$p.CommandLine
  if ($cmd) {
    Write-Output ("PROC`+"`t"+`" + $p.ProcessId + "`+"`t"+`" + $p.ParentProcessId + "`+"`t"+`" + $cmd)
  }
}
if (Test-Path -LiteralPath $root -PathType Container) {
  foreach ($taskEntry in (Get-ChildItem -LiteralPath $root -Directory -Filter 'task-*' -ErrorAction SilentlyContinue)) {
    $sessionsRoot = Join-Path $taskEntry.FullName '.kandev\sessions'
    if (-not (Test-Path -LiteralPath $sessionsRoot -PathType Container)) { continue }
    foreach ($sessionEntry in (Get-ChildItem -LiteralPath $sessionsRoot -Directory -ErrorAction SilentlyContinue)) {
      $pidFile = Join-Path $sessionEntry.FullName 'agentctl.pid'
      if (-not (Test-Path -LiteralPath $pidFile -PathType Leaf)) { continue }
      $val = ''
      try { $val = [System.IO.File]::ReadAllText($pidFile).Trim() } catch {}
      Write-Output ("PIDFILE`+"`t"+`" + $taskEntry.Name + "`+"`t"+`" + $sessionEntry.Name + "`+"`t"+`" + $val)
    }
  }
}`, powerShellQuote(tasksRoot))
	return script
}

func sshWindowsOrphanTaskIDFromWorkdir(tasksRoot, workdir string) (string, string, bool) {
	cleanRoot := cleanWindowsRemotePath(tasksRoot)
	cleanWorkdir := cleanWindowsRemotePath(workdir)
	prefix := cleanRoot + `\task-`
	if len(cleanWorkdir) <= len(prefix) || !strings.EqualFold(cleanWorkdir[:len(prefix)], prefix) {
		return "", "", false
	}
	id := cleanWorkdir[len(prefix):]
	if id == "" || strings.ContainsAny(id, `\/`) {
		return "", "", false
	}
	return "task-" + id, id, true
}
