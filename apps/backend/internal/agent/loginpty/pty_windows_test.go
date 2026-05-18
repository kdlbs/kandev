//go:build windows

package loginpty

import (
	"os/exec"
	"strings"
	"testing"
)

// TestWindowsPTY_DoubleCloseSafe pins down the sync.Once guard on
// windowsPTY.Close. The upstream UserExistsError/conpty library has no
// internal synchronization — a second Close would double-free the
// underlying Windows handles and trigger STATUS_HEAP_CORRUPTION
// (0xC0000374), the same crash class as the original #894 fix in the
// agentctl process package. We deliberately keep this implementation
// duplicated from internal/agentctl/server/process/pty_windows.go (the
// kandev backend shouldn't reach into the sidecar's internals), so the
// safety guard is independently verified here.
func TestWindowsPTY_DoubleCloseSafe(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "exit", "0")
	pty, err := startPTYWithSize(cmd, 80, 24)
	if err != nil {
		t.Fatalf("startPTYWithSize: %v", err)
	}

	if err := pty.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// Second Close must not panic, must not double-free, and must return
	// the same memoized error value as the first call.
	if err := pty.Close(); err != nil {
		t.Fatalf("second Close returned error: %v (expected nil from sync.Once)", err)
	}
}

// TestResolveConPtyCmdLine_CmdShimIsWrapped mirrors the agentctl/process
// package's equivalent test. The CreateProcessW/PATHEXT/.cmd-shim issue
// is identical for login PTY sessions: a bare "claude" or "gemini" Args[0]
// wouldn't reach the resolver, and the .cmd resolved via cmd.Path can't
// run under CreateProcess without the cmd.exe /c wrapper.
func TestResolveConPtyCmdLine_CmdShimIsWrapped(t *testing.T) {
	cmd := &exec.Cmd{
		Path: `C:\Users\test\AppData\Roaming\npm\claude.cmd`,
		Args: []string{"claude"},
	}
	got := resolveConPtyCmdLine(cmd)
	want := `cmd.exe /c C:\Users\test\AppData\Roaming\npm\claude.cmd`
	if got != want {
		t.Errorf("resolveConPtyCmdLine(.cmd shim):\n  got:  %s\n  want: %s", got, want)
	}
}

// TestResolveConPtyCmdLine_ExePassesThrough — .exe binaries should NOT
// gain an intermediate cmd.exe wrapper; that would needlessly fork
// another process and obscure the real PID for signal handling.
func TestResolveConPtyCmdLine_ExePassesThrough(t *testing.T) {
	cmd := &exec.Cmd{
		Path: `C:\Program Files\Git\bin\bash.exe`,
		Args: []string{"bash", "-l"},
	}
	got := resolveConPtyCmdLine(cmd)
	if strings.HasPrefix(got, "cmd.exe /c ") {
		t.Errorf("resolveConPtyCmdLine wrapped a non-batch executable: %s", got)
	}
	if !strings.Contains(got, cmd.Path) {
		t.Errorf("resolveConPtyCmdLine should substitute resolved cmd.Path %q for Args[0], got: %s", cmd.Path, got)
	}
}
