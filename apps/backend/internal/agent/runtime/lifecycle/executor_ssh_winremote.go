package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	windowsProbePrefix         = "KANDEV_WIN_INFO\t"
	windowsProbeScript         = "$arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }; Write-Output ('KANDEV_WIN_INFO' + \"`tWindows_NT`t\" + $arch + \"`t\" + [System.Environment]::OSVersion.VersionString)"
	windowsLivenessAliveToken  = "ALIVE"
	windowsLivenessAbsentToken = "ABSENT"
	windowsPidDirMissingToken  = "DIR_MISSING"
	windowsPidFileMissingToken = "PIDFILE_MISSING"
	windowsPidPrefix           = "PID:"
	windowsCmdlinePrefix       = "CMDLINE:"
	windowsGitVersionScript    = "try { $v = (git --version 2>$null | Out-String).Trim(); if ($v) { Write-Output $v } } catch {}"
)

func detectWindowsRemoteInfo(ctx context.Context, client *ssh.Client) (*SSHRemoteInfo, bool) {
	out, _, err := runPowerShell(ctx, client, defaultWindowsLoginShell, windowsProbeScript, false)
	if err != nil {
		return nil, false
	}
	info, ok := parseWindowsRemoteProbeOutput(out)
	if !ok {
		return nil, false
	}
	if gitOut, _, gitErr := runPowerShell(ctx, client, defaultWindowsLoginShell, windowsGitVersionScript, true); gitErr == nil {
		info.GitVer = strings.TrimSpace(gitOut)
	}
	return info, true
}

func parseWindowsRemoteProbeOutput(output string) (*SSHRemoteInfo, bool) {
	for _, line := range strings.Split(output, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), windowsProbePrefix)
		if !ok {
			continue
		}
		osName, rest, _ := strings.Cut(rest, "\t")
		arch, version, _ := strings.Cut(rest, "\t")
		osName, arch = strings.TrimSpace(osName), strings.TrimSpace(arch)
		platform, _ := normalizeSSHRemotePlatform(osName, arch)
		return &SSHRemoteInfo{
			UnameAll: strings.TrimSpace(strings.TrimSpace(version) + " " + arch),
			OS:       osName,
			Arch:     arch,
			Platform: platform,
		}, true
	}
	return nil, false
}

func expandWindowsRemoteHome(ctx context.Context, client *ssh.Client, rawPath string) (string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if isWindowsRemotePath(trimmed) {
		return cleanWindowsRemotePath(trimmed), nil
	}
	suffix, needsHome := trimWindowsHomePrefix(trimmed)
	if !needsHome {
		return trimmed, nil
	}
	script := "$h = if ($env:USERPROFILE) { $env:USERPROFILE } else { $HOME }; if ($h) { [Console]::Out.Write($h) }"
	out, _, err := runPowerShell(ctx, client, defaultWindowsLoginShell, script, false)
	if err != nil {
		return "", fmt.Errorf("ssh: resolve Windows home directory: %w", err)
	}
	home := cleanWindowsRemotePath(strings.TrimSpace(out))
	if home == "" || !isWindowsRemotePath(home) {
		return "", errors.New("ssh: remote Windows home directory is empty or invalid")
	}
	if suffix == "" {
		return home, nil
	}
	return joinWindowsRemotePath(home, suffix), nil
}

// trimWindowsHomePrefix splits a path that starts with ~, %USERPROFILE%, or
// $HOME into the part below the home directory.
func trimWindowsHomePrefix(rawPath string) (string, bool) {
	for _, home := range []string{"~", "%USERPROFILE%", "$HOME"} {
		if len(rawPath) < len(home) || !strings.EqualFold(rawPath[:len(home)], home) {
			continue
		}
		rest := rawPath[len(home):]
		if rest == "" {
			return "", true
		}
		if rest[0] == '\\' || rest[0] == '/' {
			return rest[1:], true
		}
	}
	return "", false
}

// readWindowsAgentctlSha returns the recorded sha256 of the uploaded helper. It
// returns "" when the sidecar is missing, or when remoteBin is set and that
// file is missing or empty.
func readWindowsAgentctlSha(ctx context.Context, client *ssh.Client, remoteShaFile, remoteBin string) (string, error) {
	script := fmt.Sprintf(
		"$sha = %s; $bin = %s; if ((Test-Path -LiteralPath $sha -PathType Leaf) -and (-not $bin -or ((Test-Path -LiteralPath $bin -PathType Leaf) -and (Get-Item -LiteralPath $bin).Length -gt 0))) { [Console]::Out.Write([System.IO.File]::ReadAllText($sha).Trim()) }",
		powerShellQuote(remoteShaFile),
		powerShellQuote(remoteBin),
	)
	out, _, err := runPowerShell(ctx, client, defaultWindowsLoginShell, script, false)
	return strings.TrimSpace(out), err
}

func checkWindowsAgentctlCached(ctx context.Context, client *ssh.Client, remoteShaFile, localSha string) (bool, error) {
	sha, err := readWindowsAgentctlSha(ctx, client, remoteShaFile, "")
	if err != nil {
		return false, fmt.Errorf("ssh: read remote Windows agentctl sha256: %w", err)
	}
	return sha == localSha, nil
}

func ensureWindowsAgentctlOnHost(
	ctx context.Context,
	client *ssh.Client,
	localSha string,
	localData []byte,
	localPath, remoteBin, remoteShaFile string,
	log *logger.Logger,
) (string, error) {
	if sha, err := readWindowsAgentctlSha(ctx, client, remoteShaFile, remoteBin); err == nil && sha == localSha {
		log.Debug("agentctl.exe already up-to-date on Windows remote", zap.String("sha256", localSha))
		return remoteBin, nil
	}

	log.Info("uploading agentctl.exe to Windows remote",
		zap.String("local_path", localPath),
		zap.String("remote_path", remoteBin),
		zap.String("sha256", localSha),
		zap.Int("bytes", len(localData)))

	if err := sftpReplaceWindowsFile(client, remoteBin, localData); err != nil {
		return "", fmt.Errorf("ssh: upload Windows agentctl.exe: %w", err)
	}
	if err := sftpReplaceWindowsFile(client, remoteShaFile, []byte(localSha+"\n")); err != nil {
		return "", fmt.Errorf("ssh: upload Windows agentctl.exe sha256: %w", err)
	}
	return remoteBin, nil
}

func probeWindowsRemoteBinary(ctx context.Context, client *ssh.Client, shell, binaryName string) (string, error) {
	script := fmt.Sprintf(
		"$cmd = Get-Command -Name %s -CommandType Application,ExternalScript -ErrorAction SilentlyContinue | Select-Object -First 1; if ($null -ne $cmd) { $p = if ($cmd.Path) { $cmd.Path } else { $cmd.Source }; if ($p) { [Console]::Out.Write($p) } }",
		powerShellQuote(binaryName),
	)
	out, _, err := runPowerShell(ctx, client, shell, script, true)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func ensureWindowsRemoteTaskDir(ctx context.Context, client *ssh.Client, workdirRoot, taskDirName string) (string, error) {
	root, err := expandWindowsRemoteHome(ctx, client, workdirRoot)
	if err != nil {
		return "", err
	}
	if !isWindowsRemotePath(root) {
		return "", fmt.Errorf("ssh: workdir root %q is not an absolute Windows path; use a drive path such as C:\\kandev, a UNC path, or a path under ~", workdirRoot)
	}
	taskDir := joinWindowsRemotePath(root, "tasks", taskDirName)
	script := fmt.Sprintf("[System.IO.Directory]::CreateDirectory(%s) | Out-Null", powerShellQuote(taskDir))
	if _, _, err := runPowerShell(ctx, client, defaultWindowsLoginShell, script, false); err != nil {
		return "", fmt.Errorf("ssh: mkdir Windows task dir %s: %w", taskDir, err)
	}
	return taskDir, nil
}

func ensureReuseRequiredWindowsRemoteTaskDirExists(ctx context.Context, client *ssh.Client, taskDir string) error {
	script := fmt.Sprintf("if (-not (Test-Path -LiteralPath %s -PathType Container)) { exit 1 }", powerShellQuote(cleanWindowsRemotePath(taskDir)))
	if _, _, err := runPowerShell(ctx, client, defaultWindowsLoginShell, script, false); err != nil {
		return fmt.Errorf("%w: remote task directory is unavailable", models.ErrWorkspaceReuseUnsafe)
	}
	return nil
}

func ensureWindowsRemoteSessionDir(ctx context.Context, client *ssh.Client, taskDir, sessionID string) (string, error) {
	cleanTaskDir := cleanWindowsRemotePath(taskDir)
	sessionDir := joinWindowsRemotePath(cleanTaskDir, ".kandev", "sessions", sessionID)
	script := fmt.Sprintf(
		"$td = %s; $sd = %s; if (-not (Test-Path -LiteralPath $td -PathType Container)) { throw 'task directory missing' }; [System.IO.Directory]::CreateDirectory($sd) | Out-Null",
		powerShellQuote(cleanTaskDir),
		powerShellQuote(sessionDir),
	)
	if _, _, err := runPowerShell(ctx, client, defaultWindowsLoginShell, script, false); err != nil {
		return "", fmt.Errorf("ssh: mkdir Windows session dir: %w", err)
	}
	return sessionDir, nil
}

func readRemoteWindowsAgentctlLog(ctx context.Context, client *ssh.Client, sessionDir string) string {
	cleanSessionDir := cleanWindowsRemotePath(sessionDir)
	script := fmt.Sprintf(`$sd = %s
$p = Join-Path $sd 'agentctl.log'
if (Test-Path -LiteralPath $p -PathType Leaf) {
  $fs = [System.IO.FileStream]::new($p, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
  try {
    $sr = [System.IO.StreamReader]::new($fs, [System.Text.Encoding]::UTF8)
    $text = $sr.ReadToEnd()
    if ($text) { [Console]::Out.Write($text); if (-not $text.EndsWith("`+"`n"+`")) { [Console]::Out.Write("`+"`n"+`") } }
  } finally {
    $fs.Dispose()
  }
}`, powerShellQuote(cleanSessionDir))
	out, _, _ := runPowerShell(ctx, client, defaultWindowsLoginShell, script, false)
	return out
}

func readWindowsSSHRecoveryControlPortFromLog(ctx context.Context, client *ssh.Client, sessionDir string) (int, error) {
	cleanSessionDir := cleanWindowsRemotePath(sessionDir)
	script := fmt.Sprintf(`$p = Join-Path %s 'agentctl.log'
if (-not (Test-Path -LiteralPath $p -PathType Leaf)) { throw 'agentctl.log missing' }
$fs = [System.IO.FileStream]::new($p, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
try {
  $len = $fs.Length
  $headLen = [Math]::Min(32768L, $len)
  $headBuf = New-Object byte[] $headLen
  [void]$fs.Read($headBuf, 0, [int]$headLen)
  $headText = [System.Text.Encoding]::UTF8.GetString($headBuf)
  $tailLen = [Math]::Min(32768L, $len)
  $fs.Seek($len - $tailLen, [System.IO.SeekOrigin]::Begin) | Out-Null
  $tailBuf = New-Object byte[] $tailLen
  [void]$fs.Read($tailBuf, 0, [int]$tailLen)
  $tailText = [System.Text.Encoding]::UTF8.GetString($tailBuf)
  [Console]::Out.Write($headText + "`+"`n"+`" + $tailText)
} finally {
  $fs.Dispose()
}`, powerShellQuote(cleanSessionDir))
	output, _, err := runPowerShell(ctx, client, defaultWindowsLoginShell, script, false)
	if err != nil {
		return 0, fmt.Errorf("read exact Windows agentctl session log: %w", err)
	}
	return latestSSHAgentctlControlPort(output)
}

func probeRemoteWindowsAgentctlLiveness(ctx context.Context, client *ssh.Client, pid int) (bool, error) {
	script := fmt.Sprintf(
		"$p = Get-Process -Id %d -ErrorAction SilentlyContinue; if ($null -ne $p -and -not $p.HasExited) { Write-Output '%s' } else { Write-Output '%s' }",
		pid, windowsLivenessAliveToken, windowsLivenessAbsentToken,
	)
	stdout, stderr, err := runPowerShell(ctx, client, defaultWindowsLoginShell, script, false)
	if err != nil {
		return false, remoteProcessProbeError("Get-Process", pid, err, stderr)
	}
	switch strings.TrimSpace(stdout) {
	case windowsLivenessAliveToken:
		return true, nil
	case windowsLivenessAbsentToken:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected Windows process liveness output for pid %d: %q", pid, strings.TrimSpace(stdout))
	}
}

// verifyRemoteWindowsAgentctlIdentity is verifyRemoteAgentctlIdentity for a
// Windows host. Only an agentctl.exe process can match, which leaves out the
// cmd.exe launch wrapper that carries the same --workdir text.
func verifyRemoteWindowsAgentctlIdentity(ctx context.Context, client *ssh.Client, pid int, sessionDir, taskDir string) (bool, error) {
	script := fmt.Sprintf(
		"$proc = Get-CimInstance -ClassName Win32_Process -Filter \"ProcessId = %d AND Name = 'agentctl.exe'\" -ErrorAction Stop; if ($null -eq $proc) { Write-Output '%s' } else { Write-Output ('%s' + [string]$proc.CommandLine) }",
		pid, windowsLivenessAbsentToken, windowsCmdlinePrefix,
	)
	stdout, stderr, err := runPowerShell(ctx, client, defaultWindowsLoginShell, script, false)
	if err != nil {
		return false, remoteProcessProbeError("Win32_Process", pid, err, stderr)
	}
	trimmed := strings.TrimSpace(stdout)
	if trimmed == windowsLivenessAbsentToken {
		return verifyAbandonedRemoteAgentctlDirectory(ctx, client, pid, sessionDir)
	}
	cmdline, ok := strings.CutPrefix(trimmed, windowsCmdlinePrefix)
	if !ok {
		return false, fmt.Errorf("unexpected Windows process identity output for pid %d: %q", pid, trimmed)
	}
	return confirmRemoteAgentctlIdentity(ctx, client, pid, sessionDir, remoteWindowsAgentctlCommandLineMatches(cmdline, taskDir))
}

func readRemoteWindowsAgentctlPidFile(ctx context.Context, client *ssh.Client, sessionDir string) (int, bool, error) {
	cleanSessionDir := cleanWindowsRemotePath(sessionDir)
	script := fmt.Sprintf(`$sd = %s
$pf = Join-Path $sd 'agentctl.pid'
if (-not (Test-Path -LiteralPath $sd -PathType Container)) {
  Write-Output '%s'
  exit 0
}
if (-not (Test-Path -LiteralPath $pf -PathType Leaf)) {
  Write-Output '%s'
  exit 0
}
Write-Output ('%s' + [System.IO.File]::ReadAllText($pf).Trim())`,
		powerShellQuote(cleanSessionDir),
		windowsPidDirMissingToken,
		windowsPidFileMissingToken,
		windowsPidPrefix,
	)
	stdout, stderr, err := runPowerShell(ctx, client, defaultWindowsLoginShell, script, false)
	if err != nil {
		return 0, false, fmt.Errorf("remote Windows pidfile probe %s failed: %w (stderr: %s)", cleanSessionDir, err, strings.TrimSpace(stderr))
	}
	trimmed := strings.TrimSpace(stdout)
	switch {
	case trimmed == windowsPidDirMissingToken:
		return 0, false, nil
	case trimmed == windowsPidFileMissingToken:
		return 0, false, fmt.Errorf("remote Windows pidfile %s\\agentctl.pid is missing", cleanSessionDir)
	case strings.HasPrefix(trimmed, windowsPidPrefix):
		rawPID := strings.TrimSpace(strings.TrimPrefix(trimmed, windowsPidPrefix))
		parsed, convErr := strconv.Atoi(rawPID)
		if convErr != nil || parsed <= 0 {
			return 0, false, fmt.Errorf("remote Windows pidfile %s\\agentctl.pid contains invalid pid %q", cleanSessionDir, rawPID)
		}
		return parsed, true, nil
	default:
		return 0, false, fmt.Errorf("unexpected Windows pidfile probe output for %s: %q", cleanSessionDir, trimmed)
	}
}

func remoteWindowsAgentctlCommandLineMatches(commandLine, taskDir string) bool {
	workdir, ok := windowsAgentctlWorkdir(commandLine)
	return ok && strings.TrimSpace(taskDir) != "" &&
		strings.EqualFold(cleanWindowsRemotePath(workdir), cleanWindowsRemotePath(taskDir))
}

// windowsAgentctlWorkdir returns the --workdir value of an agentctl.exe command
// line. The launch always writes it as --workdir "<path>", and a Windows path
// cannot contain a double quote.
func windowsAgentctlWorkdir(commandLine string) (string, bool) {
	_, rest, ok := strings.Cut(commandLine, `--workdir "`)
	if !ok {
		return "", false
	}
	workdir, _, ok := strings.Cut(rest, `"`)
	return workdir, ok && workdir != ""
}

func removeWindowsRemoteDirScript(cleanDir string) string {
	if strings.TrimSpace(cleanDir) == "" {
		return ""
	}
	return fmt.Sprintf(`$dirToRemove = %s
if (Test-Path -LiteralPath $dirToRemove) {
  for ($rmAttempt = 0; $rmAttempt -lt 10; $rmAttempt++) {
    try {
      Remove-Item -LiteralPath $dirToRemove -Recurse -Force -ErrorAction Stop
      break
    } catch {
      if ($rmAttempt -eq 9) { throw }
      Start-Sleep -Milliseconds 50
    }
  }
  if (Test-Path -LiteralPath $dirToRemove) {
    throw "failed to remove Windows directory $dirToRemove"
  }
}`, powerShellQuote(cleanDir))
}

func runRemoteWindowsSSHReconcile(ctx context.Context, client *ssh.Client, bin string, body []byte) (string, string, bool, error) {
	cleanBin := cleanWindowsRemotePath(bin)
	cmd, err := newPowerShellCommand(defaultWindowsLoginShell, powerShellEnvelope{
		Script: "$KandevInput | & " + powerShellQuote(cleanBin) + " ssh-reconcile",
		Input:  string(body),
	})
	if err != nil {
		return "", "", false, err
	}
	return runSSHCommandStdinBounded(ctx, client, cmd.line, strings.NewReader(cmd.stdin), maxSSHReconcileResponseBytes)
}
