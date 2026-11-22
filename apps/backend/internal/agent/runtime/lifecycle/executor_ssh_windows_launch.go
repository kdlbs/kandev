package lifecycle

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/common/logger"
)

// windowsAgentctlLaunchScript starts agentctl.exe through WMI. Win32 OpenSSH
// ends every process of an exec channel when the channel closes; a process
// created by Win32_Process.Create belongs to the WMI service instead. WMI
// replaces the child environment, so the script passes the full launching
// environment plus AGENTCTL_PORT. agentctl has no log-file option, so a
// cmd.exe wrapper redirects both streams. The script records and prints the
// agentctl.exe PID, which launch-receipt validation compares with the PID
// agentctl reports.
func windowsAgentctlLaunchScript(agentctlBin, workspacePath, sessionDir string, port int) string {
	cleanSessionDir := cleanWindowsRemotePath(sessionDir)
	cleanWorkdir := cleanWindowsRemotePath(workspacePath)
	logPath := joinWindowsRemotePath(cleanSessionDir, "agentctl.log")
	wrapper := "cmd.exe /d /s /c \"" +
		quoteWindowsArgAlways(cleanWindowsRemotePath(agentctlBin)) +
		" --workdir " + quoteWindowsArgAlways(cleanWorkdir) +
		" > " + quoteWindowsArgAlways(logPath) + " 2>&1\""
	return fmt.Sprintf(`$sessionDir = %[1]s
$workspacePath = %[2]s
$logPath = %[3]s
$pidPath = Join-Path $sessionDir 'agentctl.pid'
[System.IO.Directory]::CreateDirectory($sessionDir) | Out-Null
[System.IO.File]::WriteAllText($logPath, '')
$childEnv = @{}
foreach ($entry in [Environment]::GetEnvironmentVariables('Process').GetEnumerator()) {
  $childEnv[[string]$entry.Key] = [string]$entry.Value
}
$childEnv['AGENTCTL_PORT'] = '%[4]d'
$envBlock = [string[]]@($childEnv.GetEnumerator() | ForEach-Object { $_.Key + '=' + $_.Value })
$startup = New-CimInstance -ClassName Win32_ProcessStartup -ClientOnly -Property @{ ShowWindow = [uint16]0; EnvironmentVariables = $envBlock }
$result = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{ CommandLine = %[5]s; CurrentDirectory = $workspacePath; ProcessStartupInformation = $startup }
if ($result.ReturnValue -ne 0) {
  [Console]::Error.WriteLine("Win32_Process.Create returned $($result.ReturnValue)")
  exit 1
}
$wrapperPid = [int]$result.ProcessId
$agentPid = 0
for ($attempt = 0; $attempt -lt 100; $attempt++) {
  $child = Get-CimInstance -ClassName Win32_Process -Filter "ParentProcessId = $wrapperPid AND Name = 'agentctl.exe'" -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($null -ne $child) {
    $agentPid = [int]$child.ProcessId
    break
  }
  if ($null -eq (Get-Process -Id $wrapperPid -ErrorAction SilentlyContinue)) { break }
  Start-Sleep -Milliseconds 100
}
if ($agentPid -eq 0) {
  if ($null -ne (Get-Process -Id $wrapperPid -ErrorAction SilentlyContinue)) {
    try { & taskkill.exe /PID $wrapperPid /T /F *>$null } catch {}
  }
  [Console]::Error.WriteLine("agentctl.exe did not start under wrapper pid $wrapperPid")
  $tail = Get-Content -LiteralPath $logPath -Tail 25 -ErrorAction SilentlyContinue
  if ($tail) { [Console]::Error.WriteLine(($tail -join [Environment]::NewLine)) }
  exit 1
}
[System.IO.File]::WriteAllText($pidPath, "$agentPid" + [Environment]::NewLine)
Write-Output $agentPid
`,
		powerShellQuote(cleanSessionDir),
		powerShellQuote(cleanWorkdir),
		powerShellQuote(logPath),
		port,
		powerShellQuote(wrapper),
	)
}

func startRemoteWindowsAgentctlOnPort(
	ctx context.Context,
	client *ssh.Client,
	shell, agentctlBin, workspacePath, sessionDir string,
	env map[string]string,
	port int,
	log *logger.Logger,
) (int, error) {
	script := windowsAgentctlLaunchScript(agentctlBin, workspacePath, sessionDir, port)
	out, stderr, err := runPowerShellEnvelope(ctx, client, shell, powerShellEnvelope{Login: true, Env: env, Script: script})
	if err != nil {
		return 0, fmt.Errorf("ssh: launch Windows agentctl.exe: %w (stderr: %s)", err, strings.TrimSpace(stderr))
	}
	pid, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("ssh: Windows agentctl launch returned non-numeric pid %q", out)
	}
	return awaitRemoteAgentctlReadyForPlatform(
		ctx, client, cleanWindowsRemotePath(sessionDir), port, pid, sshAgentctlReadyTimeout, sshAgentctlReadyPoll,
		SSHRemotePlatform{GOOS: sshRemoteGOOSWindows}, log,
	)
}
