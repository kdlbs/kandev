package lifecycle

import (
	"strings"
	"testing"
)

// @covers AC-EXECUTORS-SSH-WINDOWS-001.13
func TestWindowsOwnedAgentctlKillScriptHoldsHandleBeforeIdentityCheck(t *testing.T) {
	script := windowsOwnedAgentctlKillScript(4242, `C:\Users\kandev\tasks\task-1`)
	hold := strings.Index(script, "$targetHandle = Get-Process -Id $targetPid")
	query := strings.Index(script, `Get-CimInstance -ClassName Win32_Process -Filter "ProcessId = $targetPid"`)
	kill := strings.Index(script, "$targetHandle.Kill()")
	if hold < 0 || query < 0 || kill < 0 {
		t.Fatalf("kill script lacks the held-handle sequence: hold=%d query=%d kill=%d", hold, query, kill)
	}
	if hold >= query || query >= kill {
		t.Fatalf("kill script must hold the handle (%d) before the identity query (%d) and kill through it (%d)",
			hold, query, kill)
	}
	if strings.Contains(script, "Stop-Process") {
		t.Fatal("kill script must not signal the bare PID with Stop-Process")
	}
	if n := strings.Count(script, "Get-Process -Id $targetPid"); n != 1 {
		t.Fatalf("kill script looks up the PID %d times; only the handle acquisition may do so", n)
	}
}
