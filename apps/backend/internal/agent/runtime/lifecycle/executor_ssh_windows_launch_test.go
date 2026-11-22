package lifecycle

import (
	"context"
	"strings"
	"testing"
)

const (
	testWinAgentctl  = `C:\Users\Kandev User\.kandev\bin\agentctl.exe`
	testWinTaskDir   = `C:\Users\Kandev User\.kandev\tasks\task 1`
	testWinSessionDr = `C:\Users\Kandev User\.kandev\tasks\task 1\.kandev\sessions\s1`
)

// @covers AC-EXECUTORS-SSH-WINDOWS-001.9
// @covers AC-EXECUTORS-SSH-WINDOWS-001.11
func TestSSHWindowsLaunchUsesWMIWithMergedEnvironment(t *testing.T) {
	script := windowsAgentctlLaunchScript(testWinAgentctl, testWinTaskDir, testWinSessionDr, 45123)
	wantWrapper := `cmd.exe /d /s /c ""C:\Users\Kandev User\.kandev\bin\agentctl.exe" --workdir "C:\Users\Kandev User\.kandev\tasks\task 1" > "C:\Users\Kandev User\.kandev\tasks\task 1\.kandev\sessions\s1\agentctl.log" 2>&1"`
	for _, want := range []string{
		"Invoke-CimMethod -ClassName Win32_Process -MethodName Create",
		"[Environment]::GetEnvironmentVariables('Process')",
		"$childEnv['AGENTCTL_PORT'] = '45123'",
		"EnvironmentVariables = $envBlock",
		powerShellQuote(wantWrapper),
		"CurrentDirectory = $workspacePath",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("launch script missing %q", want)
		}
	}
	if strings.Contains(script, "Start-Process") {
		t.Error("launch script must not use Start-Process; OpenSSH ends its children with the channel")
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.13
func TestSSHWindowsLaunchRecordsAgentctlChildPID(t *testing.T) {
	script := windowsAgentctlLaunchScript(testWinAgentctl, testWinTaskDir, testWinSessionDr, 45123)
	for _, want := range []string{
		"ParentProcessId = $wrapperPid AND Name = 'agentctl.exe'",
		`[System.IO.File]::WriteAllText($pidPath, "$agentPid" + [Environment]::NewLine)`,
		"Write-Output $agentPid",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("launch script missing %q", want)
		}
	}
	if strings.Contains(script, "Write-Output $wrapperPid") || strings.Contains(script, `"$wrapperPid" + [Environment]::NewLine`) {
		t.Error("launch script must record the agentctl.exe PID, not the wrapper PID")
	}
}

func TestSSHWindowsLaunchFailsOnWMIReturnCode(t *testing.T) {
	script := windowsAgentctlLaunchScript(testWinAgentctl, testWinTaskDir, testWinSessionDr, 45123)
	if !strings.Contains(script, "if ($result.ReturnValue -ne 0)") || !strings.Contains(script, "Win32_Process.Create returned") {
		t.Fatal("launch script must fail on a non-zero Win32_Process.Create return value")
	}
	server := newFakeSSHServer(t, func(command, stdin string) sshExecResult {
		if _, ok := decodeTestPowerShell(command, stdin); !ok {
			return sshFail("expected the PowerShell loader")
		}
		return sshFail("Win32_Process.Create returned 9")
	})
	_, err := startRemoteWindowsAgentctlOnPort(context.Background(), server.dial(t), "powershell",
		testWinAgentctl, testWinTaskDir, testWinSessionDr, nil, 45123, newTestLogger())
	if err == nil || !strings.Contains(err.Error(), "Win32_Process.Create returned 9") {
		t.Fatalf("launch error = %v, want the WMI return code", err)
	}
}

func TestSSHWindowsLaunchStopsWrapperWhenChildMissing(t *testing.T) {
	script := windowsAgentctlLaunchScript(testWinAgentctl, testWinTaskDir, testWinSessionDr, 45123)
	_, missing, found := strings.Cut(script, "if ($agentPid -eq 0) {")
	if !found {
		t.Fatal("launch script has no missing-child branch")
	}
	for _, want := range []string{
		"try { & taskkill.exe /PID $wrapperPid /T /F *>$null } catch {}",
		"Get-Content -LiteralPath $logPath -Tail 25",
		"agentctl.exe did not start",
		"exit 1",
	} {
		if !strings.Contains(missing, want) {
			t.Errorf("missing-child branch lacks %q", want)
		}
	}
}

func TestQuoteWindowsArgAlwaysQuotes(t *testing.T) {
	cases := map[string]string{
		`C:\plain\agentctl.exe`: `"C:\plain\agentctl.exe"`,
		`C:\`:                   `"C:\\"`,
		`C:\a&b`:                `"C:\a&b"`,
		`C:\with space`:         `"C:\with space"`,
	}
	for in, want := range cases {
		if got := quoteWindowsArgAlways(in); got != want {
			t.Errorf("quoteWindowsArgAlways(%q) = %q, want %q", in, got, want)
		}
	}
}
