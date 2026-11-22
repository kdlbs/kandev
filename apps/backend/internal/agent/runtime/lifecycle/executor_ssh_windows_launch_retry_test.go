package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Windows reserves port ranges (netsh int ipv4 show excludedportrange) that
// refuse a bind with an access error. agentctl then exits before the launch
// script finds its PID, so the bind failure arrives on the launch stderr and
// must still select another port.
// @covers AC-EXECUTORS-SSH-WINDOWS-001.23
func TestSSHWindowsLaunchRetriesWhenBindFails(t *testing.T) {
	stderr := "agentctl.exe did not start under wrapper pid 4242\n" +
		`{"level":"error","msg":"HTTP server failed to bind","error":"listen tcp 127.0.0.1:45123: bind: ` +
		`An attempt was made to access a socket in a way forbidden by its access permissions."}`
	server := newFakeSSHServer(t, func(command, stdin string) sshExecResult {
		if _, ok := decodeTestPowerShell(command, stdin); !ok {
			return sshFail("expected the PowerShell loader")
		}
		return sshFail(stderr)
	})
	_, err := startRemoteWindowsAgentctlOnPort(context.Background(), server.dial(t), "powershell",
		testWinAgentctl, testWinTaskDir, testWinSessionDr, nil, 45123, newTestLogger())
	if !errors.Is(err, errSSHAgentctlPortInUse) {
		t.Fatalf("launch error = %v, want errSSHAgentctlPortInUse", err)
	}
	if !strings.Contains(err.Error(), "port 45123") || !strings.Contains(err.Error(), "forbidden by its access permissions") {
		t.Fatalf("launch error = %v, want the port and the bind detail", err)
	}
}

func TestSSHWindowsLaunchFailureWithoutBindErrorIsNotRetried(t *testing.T) {
	server := newFakeSSHServer(t, func(command, stdin string) sshExecResult {
		return sshFail("agentctl.exe did not start under wrapper pid 4242\nThe system cannot find the file specified.")
	})
	_, err := startRemoteWindowsAgentctlOnPort(context.Background(), server.dial(t), "powershell",
		testWinAgentctl, testWinTaskDir, testWinSessionDr, nil, 45123, newTestLogger())
	if err == nil || errors.Is(err, errSSHAgentctlPortInUse) {
		t.Fatalf("launch error = %v, want a non-retryable launch failure", err)
	}
}
