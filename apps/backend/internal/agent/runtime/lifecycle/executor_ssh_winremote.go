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
)

func detectWindowsRemoteInfo(ctx context.Context, client *ssh.Client) (*SSHRemoteInfo, bool) {
	for _, candidate := range []string{defaultWindowsLoginShell, windowsPowerShellCoreBinary} {
		out, _, err := runPowerShell(ctx, client, candidate, windowsProbeScript, false)
		if err != nil {
			continue
		}
		info, ok := parseWindowsRemoteProbeOutput(out)
		if !ok {
			continue
		}
		gitCmd := "try { $v = (git --version 2>$null | Out-String).Trim(); if ($v) { Write-Output $v } } catch {}"
		if gitOut, _, gitErr := runPowerShell(ctx, client, candidate, gitCmd, true); gitErr == nil {
			info.GitVer = strings.TrimSpace(gitOut)
		}
		return info, true
	}
	return nil, false
}

func parseWindowsRemoteProbeOutput(output string) (*SSHRemoteInfo, bool) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, windowsProbePrefix) {
			continue
		}
		rest := strings.TrimPrefix(line, windowsProbePrefix)
		parts := strings.SplitN(rest, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		osName := strings.TrimSpace(parts[0])
		arch := strings.TrimSpace(parts[1])
		version := ""
		if len(parts) == 3 {
			version = strings.TrimSpace(parts[2])
		}
		platform, _ := normalizeSSHRemotePlatform(osName, arch)
		unameAll := strings.TrimSpace(version + " " + arch)
		if unameAll == "" {
			unameAll = osName + " " + arch
		}
		return &SSHRemoteInfo{
			UnameAll: unameAll,
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

func trimWindowsHomePrefix(rawPath string) (string, bool) {
	switch {
	case rawPath == "~" || strings.EqualFold(rawPath, "%USERPROFILE%") || strings.EqualFold(rawPath, "$HOME"):
		return "", true
	case strings.HasPrefix(rawPath, "~/") || strings.HasPrefix(rawPath, `~\`):
		return rawPath[2:], true
	case len(rawPath) > len("%USERPROFILE%")+1 && strings.EqualFold(rawPath[:len("%USERPROFILE%")], "%USERPROFILE%") && (rawPath[len("%USERPROFILE%")] == '\\' || rawPath[len("%USERPROFILE%")] == '/'):
		return rawPath[len("%USERPROFILE%")+1:], true
	case len(rawPath) > len("$HOME")+1 && strings.EqualFold(rawPath[:len("$HOME")], "$HOME") && (rawPath[len("$HOME")] == '\\' || rawPath[len("$HOME")] == '/'):
		return rawPath[len("$HOME")+1:], true
	default:
		return "", false
	}
}

func checkWindowsAgentctlCached(ctx context.Context, client *ssh.Client, remoteShaFile, localSha string) (bool, error) {
	script := fmt.Sprintf(
		"$p = %s; if (Test-Path -LiteralPath $p -PathType Leaf) { [Console]::Out.Write([System.IO.File]::ReadAllText($p).Trim()) }",
		powerShellQuote(remoteShaFile),
	)
	out, _, err := runPowerShell(ctx, client, defaultWindowsLoginShell, script, false)
	if err != nil {
		return false, fmt.Errorf("ssh: read remote Windows agentctl sha256: %w", err)
	}
	return strings.TrimSpace(out) == localSha, nil
}

func ensureWindowsAgentctlOnHost(
	ctx context.Context,
	client *ssh.Client,
	localSha string,
	localData []byte,
	localPath, remoteBin, remoteShaFile string,
	log *logger.Logger,
) (string, error) {
	checkScript := fmt.Sprintf(
		"$sha = %s; $bin = %s; if ((Test-Path -LiteralPath $sha -PathType Leaf) -and (Test-Path -LiteralPath $bin -PathType Leaf) -and ((Get-Item -LiteralPath $bin).Length -gt 0)) { [Console]::Out.Write([System.IO.File]::ReadAllText($sha).Trim()) }",
		powerShellQuote(remoteShaFile),
		powerShellQuote(remoteBin),
	)
	if out, _, err := runPowerShell(ctx, client, defaultWindowsLoginShell, checkScript, false); err == nil {
		if strings.TrimSpace(out) == localSha {
			log.Debug("agentctl.exe already up-to-date on Windows remote", zap.String("sha256", localSha))
			return remoteBin, nil
		}
	}

	log.Info("uploading agentctl.exe to Windows remote",
		zap.String("local_path", localPath),
		zap.String("remote_path", remoteBin),
		zap.String("sha256", localSha),
		zap.Int("bytes", len(localData)))

	binDir := parentWindowsRemotePath(remoteBin)
	mkdirScript := fmt.Sprintf("[System.IO.Directory]::CreateDirectory(%s) | Out-Null", powerShellQuote(binDir))
	if _, _, err := runPowerShell(ctx, client, defaultWindowsLoginShell, mkdirScript, false); err != nil {
		return "", fmt.Errorf("ssh: mkdir for Windows agentctl.exe: %w", err)
	}
	if err := sftpReplaceWindowsFile(client, remoteBin, localData); err != nil {
		return "", fmt.Errorf("ssh: upload Windows agentctl.exe: %w", err)
	}
	if err := sftpReplaceWindowsFile(client, remoteShaFile, []byte(localSha+"\n")); err != nil {
		return "", fmt.Errorf("ssh: upload Windows agentctl.exe sha256: %w", err)
	}
	verifyScript := fmt.Sprintf(
		"$bin = %s; if (-not (Test-Path -LiteralPath $bin -PathType Leaf) -or ((Get-Item -LiteralPath $bin).Length -le 0)) { throw 'agentctl.exe missing after upload' }",
		powerShellQuote(remoteBin),
	)
	if _, _, err := runPowerShell(ctx, client, defaultWindowsLoginShell, verifyScript, false); err != nil {
		return "", fmt.Errorf("ssh: agentctl.exe not ready after upload: %w", err)
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
	if taskDirName == "" {
		return "", errors.New("ssh: task dir name is empty")
	}
	root, err := expandWindowsRemoteHome(ctx, client, workdirRoot)
	if err != nil {
		return "", err
	}
	taskDir := joinWindowsRemotePath(root, "tasks", taskDirName)
	script := fmt.Sprintf("[System.IO.Directory]::CreateDirectory(%s) | Out-Null", powerShellQuote(taskDir))
	if _, _, err := runPowerShell(ctx, client, defaultWindowsLoginShell, script, false); err != nil {
		return "", fmt.Errorf("ssh: mkdir Windows task dir %s: %w", taskDir, err)
	}
	return taskDir, nil
}

func ensureReuseRequiredWindowsRemoteTaskDirExists(ctx context.Context, client *ssh.Client, taskDir string) error {
	if strings.TrimSpace(taskDir) == "" {
		return fmt.Errorf("%w: missing remote task directory", models.ErrWorkspaceReuseUnsafe)
	}
	script := fmt.Sprintf("if (-not (Test-Path -LiteralPath %s -PathType Container)) { exit 1 }", powerShellQuote(cleanWindowsRemotePath(taskDir)))
	if _, _, err := runPowerShell(ctx, client, defaultWindowsLoginShell, script, false); err != nil {
		return fmt.Errorf("%w: remote task directory is unavailable", models.ErrWorkspaceReuseUnsafe)
	}
	return nil
}

func ensureWindowsRemoteSessionDir(ctx context.Context, client *ssh.Client, taskDir, sessionID string) (string, error) {
	if sessionID == "" {
		return "", errors.New("ssh: session ID is empty")
	}
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
	cleanSessionDir := cleanWindowsRemotePath(strings.TrimSpace(sessionDir))
	if cleanSessionDir == "" {
		return 0, errors.New("persisted agentctl session directory is unavailable")
	}
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
	if pid <= 0 {
		return false, nil
	}
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

func verifyRemoteWindowsAgentctlIdentity(ctx context.Context, client *ssh.Client, pid int, sessionDir, taskDir string) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
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
		return verifyAbandonedRemoteWindowsAgentctlDirectory(ctx, client, pid, sessionDir)
	}
	cmdline, ok := strings.CutPrefix(trimmed, windowsCmdlinePrefix)
	if !ok {
		return false, fmt.Errorf("unexpected Windows process identity output for pid %d: %q", pid, trimmed)
	}
	if !remoteWindowsAgentctlCommandLineMatches(cmdline, taskDir) {
		return verifyAbandonedRemoteWindowsAgentctlDirectory(ctx, client, pid, sessionDir)
	}
	if err := remoteWindowsAgentctlPidFileConfirms(ctx, client, sessionDir, pid); err != nil {
		return false, err
	}
	return true, nil
}

func verifyAbandonedRemoteWindowsAgentctlDirectory(ctx context.Context, client *ssh.Client, pid int, sessionDir string) (bool, error) {
	filePID, present, err := readRemoteWindowsAgentctlPidFile(ctx, client, sessionDir)
	if err != nil {
		return false, err
	}
	if !present {
		return false, nil
	}
	if filePID != pid {
		return false, fmt.Errorf("remote Windows pidfile %s\\agentctl.pid names pid %d, not %d", cleanWindowsRemotePath(sessionDir), filePID, pid)
	}
	return false, nil
}

func remoteWindowsAgentctlPidFileConfirms(ctx context.Context, client *ssh.Client, sessionDir string, pid int) error {
	filePID, present, err := readRemoteWindowsAgentctlPidFile(ctx, client, sessionDir)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("%w: %s", errSSHRemoteSessionDirectoryMissing, sessionDir)
	}
	if filePID != pid {
		return fmt.Errorf("remote Windows pidfile %s\\agentctl.pid names pid %d, not %d", cleanWindowsRemotePath(sessionDir), filePID, pid)
	}
	return nil
}

func readRemoteWindowsAgentctlPidFile(ctx context.Context, client *ssh.Client, sessionDir string) (int, bool, error) {
	cleanSessionDir := cleanWindowsRemotePath(strings.TrimSpace(sessionDir))
	if cleanSessionDir == "" {
		return 0, false, errors.New("remote Windows agentctl pidfile check: no session directory")
	}
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
	line := strings.TrimSpace(commandLine)
	if line == "" || strings.TrimSpace(taskDir) == "" {
		return false
	}
	if !strings.Contains(strings.ToLower(line), "agentctl") {
		return false
	}
	workdir, ok := extractWindowsFlagValue(line, "--workdir")
	if !ok {
		return false
	}
	return strings.EqualFold(cleanWindowsRemotePath(workdir), cleanWindowsRemotePath(taskDir))
}

func extractWindowsFlagValue(commandLine, flag string) (string, bool) {
	tokens := splitWindowsCommandLine(commandLine)
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok == flag {
			if i+1 < len(tokens) && tokens[i+1] != "" {
				return tokens[i+1], true
			}
			return "", false
		}
		if strings.HasPrefix(tok, flag+"=") {
			val := strings.TrimPrefix(tok, flag+"=")
			if val != "" {
				return val, true
			}
		}
	}
	return "", false
}

// splitWindowsCommandLine parses a Win32 command line into arguments following
// CommandLineToArgvW quoting rules (plus single-quote support if invoked via PS).
func splitWindowsCommandLine(line string) []string {
	var state windowsCmdTokenizer
	for i := 0; i < len(line); i++ {
		i = state.consume(line, i)
	}
	state.flush()
	return state.args
}

type windowsCmdTokenizer struct {
	args      []string
	cur       strings.Builder
	inToken   bool
	quoteChar byte
}

func (t *windowsCmdTokenizer) flush() {
	if t.inToken {
		t.args = append(t.args, t.cur.String())
		t.cur.Reset()
		t.inToken = false
	}
}

func (t *windowsCmdTokenizer) consume(line string, i int) int {
	ch := line[i]
	switch t.quoteChar {
	case 0:
		return t.consumeUnquoted(ch, i)
	case '\'':
		if ch == '\'' {
			t.quoteChar = 0
		} else {
			t.cur.WriteByte(ch)
		}
		return i
	default:
		return t.consumeDoubleQuoted(line, i, ch)
	}
}

func (t *windowsCmdTokenizer) consumeUnquoted(ch byte, i int) int {
	if ch == ' ' || ch == '\t' {
		t.flush()
		return i
	}
	t.inToken = true
	if ch == '"' || ch == '\'' {
		t.quoteChar = ch
		return i
	}
	t.cur.WriteByte(ch)
	return i
}

func (t *windowsCmdTokenizer) consumeDoubleQuoted(line string, i int, ch byte) int {
	if ch == '\\' {
		return t.consumeDoubleQuotedBackslashes(line, i)
	}
	if ch == '"' {
		t.quoteChar = 0
		return i
	}
	t.cur.WriteByte(ch)
	return i
}

func (t *windowsCmdTokenizer) consumeDoubleQuotedBackslashes(line string, start int) int {
	bs := 0
	i := start
	for i < len(line) && line[i] == '\\' {
		bs++
		i++
	}
	if i < len(line) && line[i] == '"' {
		t.cur.WriteString(strings.Repeat(`\`, bs/2))
		if bs%2 == 0 {
			t.quoteChar = 0
		} else {
			t.cur.WriteByte('"')
		}
		return i
	}
	t.cur.WriteString(strings.Repeat(`\`, bs))
	return i - 1
}

func stopRemoteWindowsAgentctl(ctx context.Context, client *ssh.Client, sessionDir string, pid int) error {
	_, stderr, err := runPowerShell(ctx, client, defaultWindowsLoginShell, remoteWindowsAgentctlStopScript(sessionDir, pid), false)
	if err != nil {
		if trimmed := strings.TrimSpace(stderr); trimmed != "" {
			return fmt.Errorf("%w (stderr: %s)", err, trimmed)
		}
		return err
	}
	return nil
}

func remoteWindowsAgentctlStopScript(sessionDir string, pid int) string {
	cleanSessionDir := cleanWindowsRemotePath(sessionDir)
	removeSnippet := removeWindowsRemoteDirScript(cleanSessionDir)
	if pid <= 0 {
		return removeSnippet
	}
	script := fmt.Sprintf(`$targetPid = %[1]d
$proc = Get-Process -Id $targetPid -ErrorAction SilentlyContinue
if ($null -ne $proc -and -not $proc.HasExited) {
  try { & taskkill.exe /PID $targetPid /T /F *>$null } catch {}
  try { Stop-Process -Id $targetPid -Force -ErrorAction SilentlyContinue } catch {}
  for ($attempt = 0; $attempt -lt %[2]d; $attempt++) {
    $check = Get-Process -Id $targetPid -ErrorAction SilentlyContinue
    if ($null -eq $check -or $check.HasExited) { break }
    Start-Sleep -Milliseconds 100
  }
  $still = Get-Process -Id $targetPid -ErrorAction SilentlyContinue
  if ($null -ne $still -and -not $still.HasExited) {
    [Console]::Error.WriteLine("remote Windows agentctl pid $targetPid is still running")
    exit 1
  }
}
%[3]s`, pid, sshAgentctlStopPollAttempts, removeSnippet)
	return script
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
	script := fmt.Sprintf(
		"$KandevInput | & %s ssh-reconcile; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }",
		powerShellQuote(cleanBin),
	)
	return runPowerShellEnvelopeBounded(
		ctx, client, defaultWindowsLoginShell, powerShellEnvelope{Script: script, Input: string(body)},
		maxSSHReconcileResponseBytes,
	)
}
