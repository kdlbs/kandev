package lifecycle

import (
	"context"
	"errors"
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
		"if ($result.ReturnValue -ne 0)",
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

// Windows reserves port ranges (netsh int ipv4 show excludedportrange) that
// refuse a bind with an access error. agentctl then exits before the launch
// script finds its PID, so the bind failure arrives on the launch stderr and
// must still select another port.
// @covers AC-EXECUTORS-SSH-WINDOWS-001.23
func TestSSHWindowsLaunchFailureClassification(t *testing.T) {
	bindFailure := `{"level":"error","msg":"HTTP server failed to bind","error":"listen tcp 127.0.0.1:45123: bind: ` +
		`An attempt was made to access a socket in a way forbidden by its access permissions."}`
	for _, tc := range []struct {
		name          string
		stderr        string
		wantPortInUse bool
		wantText      []string
	}{
		{
			name:          "refused bind selects another port",
			stderr:        "agentctl.exe did not start under wrapper pid 4242\n" + bindFailure,
			wantPortInUse: true,
			wantText:      []string{"port 45123", "forbidden by its access permissions"},
		},
		{
			name:     "missing executable is not retried",
			stderr:   "agentctl.exe did not start under wrapper pid 4242\nThe system cannot find the file specified.",
			wantText: []string{"cannot find the file specified"},
		},
		{
			name:     "WMI return code is reported",
			stderr:   "Win32_Process.Create returned 9",
			wantText: []string{"Win32_Process.Create returned 9"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newFakeSSHServer(t, func(command, stdin string) sshExecResult {
				if _, ok := decodeTestPowerShell(command, stdin); !ok {
					return sshFail("expected the PowerShell loader")
				}
				return sshFail(tc.stderr)
			})
			_, err := startRemoteWindowsAgentctlOnPort(context.Background(), server.dial(t), "powershell",
				testWinAgentctl, testWinTaskDir, testWinSessionDr, nil, 45123, newTestLogger())
			if err == nil || errors.Is(err, errSSHAgentctlPortInUse) != tc.wantPortInUse {
				t.Fatalf("launch error = %v, want port-in-use %v", err, tc.wantPortInUse)
			}
			for _, want := range tc.wantText {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("launch error %q missing %q", err.Error(), want)
				}
			}
		})
	}
}

func TestSSHWindowsLaunchStopsWrapperWhenChildMissing(t *testing.T) {
	script := windowsAgentctlLaunchScript(testWinAgentctl, testWinTaskDir, testWinSessionDr, 45123)
	_, missing, found := strings.Cut(script, "if ($agentPid -eq 0) {")
	if !found {
		t.Fatal("launch script has no missing-child branch")
	}
	for _, want := range []string{
		"Invoke-KandevNative { & taskkill.exe /PID $wrapperPid /T /F *>$null }",
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
		if got := quoteWindowsArg(in); got != want {
			t.Errorf("quoteWindowsArg(%q) = %q, want %q", in, got, want)
		}
	}
}
