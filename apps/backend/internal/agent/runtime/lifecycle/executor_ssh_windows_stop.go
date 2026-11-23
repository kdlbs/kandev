package lifecycle

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// stopRemoteWindowsAgentctl stops the controller recorded for sessionDir. The
// task directory is the session directory's third ancestor, and it is the
// identity the stop script checks before it signals pid.
func stopRemoteWindowsAgentctl(ctx context.Context, client *ssh.Client, sessionDir string, pid int) error {
	taskDir := ""
	if pid > 0 {
		derived, ok := windowsTaskDirFromSessionDir(sessionDir)
		if !ok {
			return fmt.Errorf("ssh: cannot verify Windows agentctl pid %d: session directory %q is not under a task directory", pid, sessionDir)
		}
		taskDir = derived
	}
	return runWindowsStopScript(ctx, client, remoteWindowsAgentctlStopScript(sessionDir, taskDir, pid))
}

func runWindowsStopScript(ctx context.Context, client *ssh.Client, script string) error {
	_, stderr, err := runPowerShell(ctx, client, defaultWindowsLoginShell, script, false)
	if err != nil {
		if trimmed := strings.TrimSpace(stderr); trimmed != "" {
			return fmt.Errorf("%w (stderr: %s)", err, trimmed)
		}
		return err
	}
	return nil
}

// windowsTaskDirFromSessionDir reverses the session layout
// <task dir>\.kandev\sessions\<session id>.
func windowsTaskDirFromSessionDir(sessionDir string) (string, bool) {
	clean := cleanWindowsRemotePath(sessionDir)
	if !isWindowsRemotePath(clean) {
		return "", false
	}
	parts := strings.Split(clean, `\`)
	if len(parts) < 4 {
		return "", false
	}
	n := len(parts)
	if !strings.EqualFold(parts[n-3], ".kandev") || !strings.EqualFold(parts[n-2], "sessions") || parts[n-1] == "" {
		return "", false
	}
	taskDir := strings.Join(parts[:n-3], `\`)
	if !isWindowsRemotePath(taskDir) {
		return "", false
	}
	return cleanWindowsRemotePath(taskDir), true
}

// remoteWindowsAgentctlStopScript stops pid when it is still the controller for
// taskDir, then removes the session directory.
func remoteWindowsAgentctlStopScript(sessionDir, taskDir string, pid int) string {
	removeSnippet := removeWindowsRemoteDirScript(cleanWindowsRemotePath(sessionDir))
	if pid <= 0 {
		return removeSnippet
	}
	return windowsOwnedAgentctlKillScript(pid, taskDir) + "\n" + removeSnippet
}

// remoteWindowsOrphanStopScript stops a proven orphan. Its session directory is
// removed only when no live controller serves the task and the pidfile is
// absent or still names pid, so a newer launch keeps its directory.
func remoteWindowsOrphanStopScript(pid int, taskDir, sessionDir string) string {
	script := windowsOwnedAgentctlKillScript(pid, taskDir)
	if strings.TrimSpace(sessionDir) == "" {
		return script
	}
	cleanSessionDir := cleanWindowsRemotePath(sessionDir)
	remove := removeWindowsRemoteDirScript(cleanSessionDir)
	return script + fmt.Sprintf(`
$sessionPidFile = Join-Path %[1]s 'agentctl.pid'
$liveOwner = @(Get-CimInstance -ClassName Win32_Process -Filter "Name = 'agentctl.exe'" -ErrorAction Stop |
  Where-Object { ([string]$_.CommandLine).IndexOf($workdirNeedle, [System.StringComparison]::OrdinalIgnoreCase) -ge 0 })
if ($liveOwner.Count -gt 0) {
  [Console]::Error.WriteLine('orphan sweep: a live agentctl serves this task; leaving its session directory')
} elseif (-not (Test-Path -LiteralPath $sessionPidFile -PathType Leaf)) {
%[2]s
} else {
  $sessionPid = ([string](Get-Content -LiteralPath $sessionPidFile -Raw -ErrorAction SilentlyContinue)).Trim()
  if ([string]::IsNullOrEmpty($sessionPid) -or $sessionPid -eq [string]$targetPid) {
%[2]s
  } else {
    [Console]::Error.WriteLine("orphan sweep: session directory now belongs to pid $sessionPid; leaving it")
  }
}`, powerShellQuote(cleanSessionDir), remove)
}

// windowsOwnedAgentctlKillScript force-stops pid and its descendants only while
// pid is agentctl.exe started with --workdir "<taskDir>". The identity check and
// the kill run in one script, so a PID that Windows reuses for another process
// is left running.
func windowsOwnedAgentctlKillScript(pid int, taskDir string) string {
	return fmt.Sprintf(`$targetPid = %[1]d
$workdirNeedle = '--workdir "' + %[2]s + '"'
try {
  $targetProc = Get-CimInstance -ClassName Win32_Process -Filter "ProcessId = $targetPid" -ErrorAction Stop
} catch {
  [Console]::Error.WriteLine("query Win32_Process for pid ${targetPid}: $($_.Exception.Message)")
  exit 1
}
$ownsTarget = ($null -ne $targetProc) -and (([string]$targetProc.Name) -ieq 'agentctl.exe') -and
  (([string]$targetProc.CommandLine).IndexOf($workdirNeedle, [System.StringComparison]::OrdinalIgnoreCase) -ge 0)
if (($null -ne $targetProc) -and -not $ownsTarget) {
  [Console]::Error.WriteLine("pid $targetPid is not this task's agentctl; leaving it running")
}
if ($ownsTarget) {
  Invoke-KandevNative { & taskkill.exe /PID $targetPid /T /F *>$null }
  try { Stop-Process -Id $targetPid -Force -ErrorAction SilentlyContinue } catch {}
  for ($attempt = 0; $attempt -lt %[3]d; $attempt++) {
    $check = Get-Process -Id $targetPid -ErrorAction SilentlyContinue
    if ($null -eq $check -or $check.HasExited) { break }
    Start-Sleep -Milliseconds 100
  }
  $still = Get-Process -Id $targetPid -ErrorAction SilentlyContinue
  if ($null -ne $still -and -not $still.HasExited) {
    [Console]::Error.WriteLine("remote Windows agentctl pid $targetPid is still running")
    exit 1
  }
}`, pid, powerShellQuote(cleanWindowsRemotePath(taskDir)), sshAgentctlStopPollAttempts)
}
