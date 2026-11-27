package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSHWindowsTaskDirFromSessionDir(t *testing.T) {
	cases := map[string]struct {
		in, want string
		ok       bool
	}{
		"drive":           {in: `C:\Users\k\.kandev\tasks\task-1\.kandev\sessions\s1`, want: `C:\Users\k\.kandev\tasks\task-1`, ok: true},
		"forward slashes": {in: `C:/Users/k/.kandev/tasks/task-1/.kandev/sessions/s1`, want: `C:\Users\k\.kandev\tasks\task-1`, ok: true},
		"unc":             {in: `\\srv\share\tasks\task-1\.kandev\sessions\s1`, want: `\\srv\share\tasks\task-1`, ok: true},
		"not a session":   {in: `C:\Users\k\.kandev\tasks\task-1`},
		"posix":           {in: `/home/k/.kandev/tasks/task-1/.kandev/sessions/s1`},
		"empty":           {in: ``},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, ok := windowsTaskDirFromSessionDir(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("windowsTaskDirFromSessionDir(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// Every Windows stop checks the task identity in the script that signals the
// PID, so nothing can reuse the PID between the check and the kill.
// @covers AC-EXECUTORS-SSH-WINDOWS-001.13
func TestSSHWindowsStopGatesKillOnTaskIdentityInOneScript(t *testing.T) {
	const sessionDir = `C:\Users\Kandev User\.kandev\tasks\task-1\.kandev\sessions\s1`
	var scripts []string
	server := newFakeSSHServer(t, func(command, stdin string) sshExecResult {
		script, ok := decodeTestPowerShell(command, stdin)
		if !ok {
			return sshFail("expected PowerShell: " + command)
		}
		scripts = append(scripts, script)
		return sshOK
	})
	if err := stopRemoteWindowsAgentctl(context.Background(), server.dial(t), sessionDir, 4242); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if len(scripts) != 1 {
		t.Fatalf("stop sent %d scripts, want exactly one", len(scripts))
	}
	script := scripts[0]
	needle := `$workdirNeedle = '--workdir "' + 'C:\Users\Kandev User\.kandev\tasks\task-1' + '"'`
	for _, want := range []string{"$targetPid = 4242", needle, "-ieq 'agentctl.exe'", "if ($ownsTarget) {", "taskkill.exe /PID $targetPid /T /F"} {
		if !strings.Contains(script, want) {
			t.Fatalf("stop script missing %q:\n%s", want, script)
		}
	}
	if strings.Index(script, "if ($ownsTarget) {") > strings.Index(script, "taskkill.exe") {
		t.Fatal("taskkill runs outside the identity gate")
	}
}

func TestSSHWindowsStopRefusesUnverifiableSessionDir(t *testing.T) {
	server := newFakeSSHServer(t, func(string, string) sshExecResult {
		t.Error("stop must not reach the host when it cannot verify the PID")
		return sshOK
	})
	err := stopRemoteWindowsAgentctl(context.Background(), server.dial(t), `C:\Users\k\somewhere`, 4242)
	if err == nil || !strings.Contains(err.Error(), "cannot verify") {
		t.Fatalf("stop error = %v, want an unverifiable-session error", err)
	}
}

// The orphan stop keeps a session directory that a newer launch has claimed.
// @covers AC-EXECUTORS-SSH-WINDOWS-001.14
func TestSSHWindowsOrphanStopGuardsSessionDir(t *testing.T) {
	script := remoteWindowsOrphanStopScript(7788, `C:\k\tasks\task-a`, `C:\k\tasks\task-a\.kandev\sessions\s1`)
	for _, want := range []string{
		"$targetPid = 7788",
		`'--workdir "' + 'C:\k\tasks\task-a' + '"'`,
		"$liveOwner = @(Get-CimInstance -ClassName Win32_Process -Filter \"Name = 'agentctl.exe'\"",
		"$sessionPid -eq [string]$targetPid",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("orphan stop script missing %q:\n%s", want, script)
		}
	}
	if !strings.Contains(remoteWindowsOrphanStopScript(7788, `C:\k\tasks\task-a`, ""), "$targetPid = 7788") ||
		strings.Contains(remoteWindowsOrphanStopScript(7788, `C:\k\tasks\task-a`, ""), "Remove-Item") {
		t.Fatal("an orphan without a claimed session must be stopped without removing any directory")
	}
}

// A PID that Windows reused for an unrelated process survives a stop.
// @covers AC-EXECUTORS-SSH-WINDOWS-001.13
func TestSSHWindowsExecStopLeavesReusedPidRunning(t *testing.T) {
	requireWindowsExecution(t)
	ctx := context.Background()
	client := newLocalWindowsSSHServer(t).dial(t)
	taskDir := filepath.Join(t.TempDir(), "kandev root", "tasks", "task-reused")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionDir, err := ensureWindowsRemoteSessionDir(ctx, client, taskDir, "s1")
	if err != nil {
		t.Fatal(err)
	}
	unrelated := exec.Command("ping", "-n", "600", "127.0.0.1")
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { killWindowsProcessTree(unrelated.Process.Pid) })

	if err := stopRemoteAgentctl(ctx, client, sessionDir, unrelated.Process.Pid); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if alive, err := probeRemoteWindowsAgentctlLiveness(ctx, client, unrelated.Process.Pid); err != nil || !alive {
		t.Fatalf("unrelated process holding the recorded PID: alive=%v err=%v, want alive", alive, err)
	}
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Fatalf("session directory remains after stop: %v", err)
	}
}
