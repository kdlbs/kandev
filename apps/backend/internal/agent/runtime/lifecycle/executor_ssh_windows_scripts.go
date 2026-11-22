package lifecycle

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/scriptengine"
	"github.com/kandev/kandev/internal/task/models"
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

func windowsBranchCheckoutPostlude() string {
	return `
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
}

func isBuiltinPOSIXSSHPrepareScript(script string) bool {
	trimmed := strings.TrimSpace(script)
	if trimmed == "" {
		return false
	}
	builtin := strings.TrimSpace(defaultSSHPrepareScript)
	return trimmed == builtin || strings.TrimSpace(repairLegacyFetchRefspec(trimmed)) == builtin
}

func withWindowsBranchCheckout(req *ExecutorCreateRequest, script string) string {
	if req == nil || selectedCheckoutIsPullRequest(req.Metadata) {
		return script
	}
	if _, ok := req.RemoteContributions[""]; ok {
		return script
	}
	return script + "\n" + windowsBranchCheckoutPostlude()
}

func (r *SSHExecutor) resolveWindowsPrepareScript(req *ExecutorCreateRequest, workspacePath, agentctlBin string) (string, error) {
	if req == nil {
		return "", fmt.Errorf("ssh: prepare request is required")
	}
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
	return r.resolveWindowsSSHScript(req, workspacePath, agentctlBin, script)
}

func (r *SSHExecutor) resolveWindowsSSHScript(req *ExecutorCreateRequest, workspacePath, agentctlBin, script string) (string, error) {
	if req == nil {
		return "", fmt.Errorf("ssh: script request is required")
	}
	cleanWorkspace := cleanWindowsRemotePath(workspacePath)
	resolver := scriptengine.NewResolver().
		WithStatic(sshStaticScriptPlaceholders()).
		WithProvider(windowsWorkspaceProvider(cleanWorkspace)).
		WithProvider(windowsGitIdentityProvider(req.Metadata)).
		WithProvider(windowsGitHubAuthProvider(req.Env)).
		WithProvider(scriptengine.AgentInstallProvider(r.collectAgentInstallScripts(req))).
		WithProvider(windowsWorktreeProvider(
			cleanWorkspace,
			getMetadataString(req.Metadata, MetadataKeyWorktreeID),
			getMetadataString(req.Metadata, MetadataKeyWorktreeBranch),
			getMetadataString(req.Metadata, MetadataKeyBaseBranch),
		)).
		WithProvider(windowsRepositoryProvider(req.Metadata))

	resolved := resolver.Resolve(script)
	if strings.Contains(resolved, "{{") {
		return "", fmt.Errorf("ssh: unresolved prepare script placeholder")
	}
	_ = agentctlBin
	return strings.TrimSpace(resolved), nil
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
		branch = strings.TrimPrefix(strings.TrimSpace(branch), "refs/heads/")
		vars["repository.branch"] = powerShellQuote(branch)
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

func sshWindowsContributionDestinationScript(workspacePath string, destination *models.ContributionDestination) string {
	remoteName := destination.ContributionRemoteName()
	return fmt.Sprintf(`$__kDestDir = %[1]s
$__kDestRemote = %[2]s
$__kDestUrl = %[3]s
$__kConfigured = (Invoke-KandevNative { & git -C $__kDestDir config --get ("remote." + $__kDestRemote + ".url") 2>$null } | Out-String).Trim()
if ($__kConfigured) {
  if ($__kConfigured -ne $__kDestUrl) { throw 'kandev: contribution destination identity conflict' }
} else {
  & git -C $__kDestDir remote add $__kDestRemote $__kDestUrl
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
& git -C $__kDestDir config --replace-all ("remote." + $__kDestRemote + ".pushurl") $__kDestUrl
$__kCurBranch = (& git -C $__kDestDir branch --show-current | Out-String).Trim()
if ($__kCurBranch) {
  & git -C $__kDestDir config ("branch." + $__kCurBranch + ".pushRemote") $__kDestRemote
}`,
		powerShellQuote(cleanWindowsRemotePath(workspacePath)),
		powerShellQuote(remoteName),
		powerShellQuote(destination.TargetRepository.RemoteURL),
	)
}

func sshWindowsRemoteContributionScript(taskDir, targetURL string, binding *models.RemoteContribution) string {
	remoteName, remoteBranch, remoteRef, refspec, suffix := sshRemoteContributionScriptValues(binding)
	return fmt.Sprintf(`$__kWs = %[1]s
$__kTargetUrl = %[2]s
$__kContribRemote = %[3]s
$__kContribUrl = %[4]s
$__kBaseBranch = %[5]s
$__kContribBranch = %[6]s
$__kContribRef = %[7]s
$__kContribRefspec = %[8]s
$__kExpectedHead = (%[9]s).ToLowerInvariant()
Invoke-KandevNative { & git -C $__kWs rev-parse --git-dir *>$null }
if ($LASTEXITCODE -eq 0) {
  $__kOrigin = (Invoke-KandevNative { & git -C $__kWs config --get remote.origin.url 2>$null } | Out-String).Trim()
  if (-not $__kOrigin) { & git -C $__kWs remote add origin $__kTargetUrl }
  elseif ($__kOrigin -ne $__kTargetUrl) { throw 'kandev: target origin identity conflict' }
} else {
  & git init -q $__kWs
  & git -C $__kWs remote add origin $__kTargetUrl
}
Invoke-KandevNative { & git -C $__kWs fetch --no-tags origin ("+refs/heads/" + $__kBaseBranch + ":refs/remotes/origin/" + $__kBaseBranch) *>$null }
if ($LASTEXITCODE -ne 0) { throw 'kandev: target base branch is unavailable' }
$__kRemoteUrl = (Invoke-KandevNative { & git -C $__kWs config --get ("remote." + $__kContribRemote + ".url") 2>$null } | Out-String).Trim()
if (-not $__kRemoteUrl) { & git -C $__kWs remote add $__kContribRemote $__kContribUrl }
elseif ($__kRemoteUrl -ne $__kContribUrl) { throw 'kandev: contribution remote identity conflict' }
Invoke-KandevNative { & git -C $__kWs fetch --no-tags $__kContribRemote $__kContribRefspec *>$null }
if ($LASTEXITCODE -ne 0) { throw 'kandev: contribution source branch is unavailable' }
$__kActualHead = (Invoke-KandevNative { & git -C $__kWs rev-parse --verify $__kContribRef 2>$null } | Out-String).Trim().ToLowerInvariant()
if (-not $__kActualHead -or ($__kActualHead -ne $__kExpectedHead)) { throw 'kandev: contribution source head changed' }
$__kCheckout = $__kContribBranch
& git -C $__kWs show-ref --verify --quiet ("refs/heads/" + $__kCheckout)
if ($LASTEXITCODE -eq 0) {
  Invoke-KandevNative { & git -C $__kWs merge-base --is-ancestor $__kExpectedHead ("refs/heads/" + $__kCheckout) *>$null }
  if ($LASTEXITCODE -ne 0) { $__kCheckout = $__kContribBranch + "-kandev-%[10]s" }
}
Invoke-KandevNative { & git -C $__kWs checkout -B $__kCheckout $__kContribRef *>$null }
Invoke-KandevNative { & git -C $__kWs branch --set-upstream-to=($__kContribRemote + "/" + $__kContribBranch) $__kCheckout *>$null }`,
		powerShellQuote(cleanWindowsRemotePath(taskDir)),
		powerShellQuote(targetURL),
		powerShellQuote(remoteName),
		powerShellQuote(binding.SourceRepository.RemoteURL),
		powerShellQuote(binding.BaseBranch),
		powerShellQuote(remoteBranch),
		powerShellQuote(remoteRef),
		powerShellQuote(refspec),
		powerShellQuote(binding.HeadSHA),
		suffix,
	)
}

func (r *SSHExecutor) verifyWindowsPrimaryCheckout(ctx context.Context, client *ssh.Client, taskDir string, req *ExecutorCreateRequest, platform SSHRemotePlatform) error {
	targetURL := getMetadataString(req.Metadata, "repository_clone_url")
	if targetURL == "" {
		return nil
	}
	shell := sshShellForRemote(req.Metadata, platform)
	cleanTask := cleanWindowsRemotePath(taskDir)
	canonCmd := fmt.Sprintf("[Console]::Out.Write((Get-Item -LiteralPath %s -Force).FullName)", powerShellQuote(cleanTask))
	canonicalTaskDir, taskDirStderr, taskDirErr := runPowerShell(ctx, client, shell, canonCmd, true)
	if taskDirErr != nil {
		r.report(req.OnProgress, "Verifying primary checkout", PrepareStepFailed, sshCommandFailureDetail(canonicalTaskDir, taskDirStderr, taskDirErr))
		return fmt.Errorf("ssh: primary repository checkout is missing")
	}
	rootCmd := fmt.Sprintf("& git -C %s rev-parse --show-toplevel; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }", powerShellQuote(cleanTask))
	root, rootStderr, rootErr := runPowerShell(ctx, client, shell, rootCmd, true)
	if rootErr != nil || !sshRemotePathsEqual(root, canonicalTaskDir, platform) {
		detail := sshCommandFailureDetail(root, rootStderr, rootErr)
		if rootErr == nil {
			detail = fmt.Sprintf("checkout root %q does not match task directory %q", strings.TrimSpace(root), strings.TrimSpace(canonicalTaskDir))
		}
		r.report(req.OnProgress, "Verifying primary checkout", PrepareStepFailed, detail)
		return fmt.Errorf("ssh: primary repository checkout is missing")
	}
	originCmd := fmt.Sprintf("& git -C %s config --get remote.origin.url; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }", powerShellQuote(cleanTask))
	origin, originStderr, originErr := runPowerShell(ctx, client, shell, originCmd, true)
	if originErr != nil || strings.TrimSpace(origin) != targetURL {
		r.report(req.OnProgress, "Verifying primary checkout", PrepareStepFailed, sshCommandFailureDetail(origin, originStderr, originErr))
		return fmt.Errorf("ssh: primary repository origin does not match configured repository")
	}
	headCmd := fmt.Sprintf("& git -C %s rev-parse --verify HEAD; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }", powerShellQuote(cleanTask))
	if head, headStderr, headErr := runPowerShell(ctx, client, shell, headCmd, true); headErr != nil {
		r.report(req.OnProgress, "Verifying primary checkout", PrepareStepFailed, sshCommandFailureDetail(head, headStderr, headErr))
		return fmt.Errorf("ssh: primary repository has no checkout")
	}
	r.report(req.OnProgress, "Verifying primary checkout", PrepareStepCompleted, strings.TrimSpace(canonicalTaskDir))
	return nil
}

func sshWindowsOrphanInventoryScript(resolvedRoot string) string {
	tasksRoot := joinWindowsRemotePath(resolvedRoot, "tasks")
	script := fmt.Sprintf(`$root = %s
$procs = Get-CimInstance -ClassName Win32_Process -ErrorAction SilentlyContinue
foreach ($p in $procs) {
  $cmd = [string]$p.CommandLine
  if ($cmd -and $cmd.ToLowerInvariant().Contains('agentctl')) {
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

func parseSSHWindowsOrphanInventory(output, resolvedRoot string) sshOrphanInventory {
	inv := sshOrphanInventory{
		pidfilesByTask: map[string]map[int]string{},
		taintedTasks:   map[string]bool{},
	}
	tasksRoot := joinWindowsRemotePath(resolvedRoot, "tasks")
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		fields := strings.SplitN(line, "\t", 4)
		switch fields[0] {
		case "PROC":
			parseSSHWindowsOrphanProcLine(&inv, tasksRoot, fields)
		case "PIDFILE":
			parseSSHOrphanPidfileLine(&inv, fields)
		}
	}
	return inv
}

func parseSSHWindowsOrphanProcLine(inv *sshOrphanInventory, tasksRoot string, fields []string) {
	if len(fields) != 4 {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil || pid <= 0 {
		return
	}
	ppid, _ := strconv.Atoi(strings.TrimSpace(fields[2]))
	command := fields[3]
	if !strings.Contains(strings.ToLower(command), "agentctl") {
		return
	}
	workdir, ok := extractWindowsFlagValue(command, "--workdir")
	if !ok {
		return
	}
	taskDir, taskID, ok := sshWindowsOrphanTaskIDFromWorkdir(tasksRoot, workdir)
	if !ok {
		return
	}
	inv.Processes = append(inv.Processes, sshOrphanProcessRecord{
		PID: pid, PPID: ppid, TaskDir: taskDir, TaskID: taskID,
	})
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

func stopSSHWindowsOrphanProcess(
	ctx context.Context,
	client *ssh.Client,
	resolvedRoot string,
	proc sshOrphanProcessRecord,
	pidfileAttributed bool,
	sessionID string,
) error {
	taskDirPath := joinWindowsRemotePath(resolvedRoot, "tasks", proc.TaskDir)
	ok, err := verifyOrphanWindowsProcessStillMatches(ctx, client, proc.PID, taskDirPath)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	var sessionDir string
	if pidfileAttributed && sessionID != "" {
		sessionDir = joinWindowsRemotePath(taskDirPath, ".kandev", "sessions", sessionID)
	}
	return stopRemoteWindowsAgentctl(ctx, client, sessionDir, proc.PID)
}

func verifyOrphanWindowsProcessStillMatches(ctx context.Context, client *ssh.Client, pid int, taskDirPath string) (bool, error) {
	script := fmt.Sprintf(
		"$proc = Get-CimInstance -ClassName Win32_Process -Filter 'ProcessId = %d' -ErrorAction Stop; if ($null -eq $proc) { Write-Output '%s' } else { Write-Output ('%s' + [string]$proc.CommandLine) }",
		pid, windowsLivenessAbsentToken, windowsCmdlinePrefix,
	)
	stdout, stderr, err := runPowerShell(ctx, client, defaultWindowsLoginShell, script, false)
	if err != nil {
		return false, remoteProcessProbeError("Win32_Process", pid, err, stderr)
	}
	trimmed := strings.TrimSpace(stdout)
	if trimmed == windowsLivenessAbsentToken {
		return true, nil
	}
	cmdline, hasPrefix := strings.CutPrefix(trimmed, windowsCmdlinePrefix)
	if !hasPrefix {
		return false, fmt.Errorf("unexpected Windows orphan recheck output: %q", trimmed)
	}
	return remoteWindowsAgentctlCommandLineMatches(cmdline, taskDirPath), nil
}
