package shell

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The task terminal panel of every remote executor reaches this session
// through agentctl's /shell/terminal routes. On Windows it must run under
// ConPTY, including in the non-interactive WMI context that SSH launches use.
// @covers AC-EXECUTORS-SSH-WINDOWS-001.20
func TestSSHWindowsShellSessionRunsUnderConPTY(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("runs a ConPTY shell on Windows")
	}
	cfg := DefaultConfig(t.TempDir())
	cfg.ShellCommand = "powershell.exe"
	session, err := NewSession(cfg, newTestLogger())
	if err != nil {
		t.Fatalf("start shell session: %v", err)
	}
	t.Cleanup(func() { _ = session.Stop() })

	if err := session.Resize(120, 40); err != nil {
		t.Fatalf("resize: %v", err)
	}
	if _, err := session.Write([]byte("Write-Output ('kandev-' + 'conpty-ok')\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if bytes.Contains(session.GetBufferedOutput(), []byte("kandev-conpty-ok")) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("shell output never showed the marker; buffered=%q", strings.TrimSpace(string(session.GetBufferedOutput())))
}
