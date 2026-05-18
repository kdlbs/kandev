//go:build windows

package process

import (
	"os/exec"
	"strings"
	"testing"
)

// TestWindowsPTY_DoubleCloseSafe guards against the double-free that crashed
// the backend on Windows when a terminal tab was closed (issue #894). Both
// InteractiveRunner.Stop and InteractiveRunner.wait close the PTY handle, so
// the wrapper must collapse the second call into a no-op — otherwise the
// underlying conpty library double-frees its Windows handles and triggers
// STATUS_HEAP_CORRUPTION.
func TestWindowsPTY_DoubleCloseSafe(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "exit", "0")
	pty, err := startPTYWithSize(cmd, 80, 24)
	if err != nil {
		t.Fatalf("startPTYWithSize: %v", err)
	}

	if err := pty.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// Second Close must not panic, must not double-free, and must return the
	// same error value as the first call.
	if err := pty.Close(); err != nil {
		t.Fatalf("second Close returned error: %v (expected nil from sync.Once)", err)
	}
}

// TestResolveConPtyCmdLine_CmdShimIsWrapped guards against the regression that
// broke passthrough/login sessions for every npm-installed CLI on Windows:
// CreateProcessW (used by ConPTY) doesn't apply PATHEXT and can't execute
// .cmd / .bat scripts directly. exec.Command resolves the binary into
// cmd.Path; we must (a) use that resolved path instead of the bare Args[0]
// and (b) wrap batch scripts with cmd.exe /c so the interpreter runs them.
func TestResolveConPtyCmdLine_CmdShimIsWrapped(t *testing.T) {
	cmd := &exec.Cmd{
		Path: `C:\Users\test\AppData\Roaming\npm\opencode.cmd`,
		Args: []string{"opencode", "--model", "big-pickle"},
	}
	got := resolveConPtyCmdLine(cmd)
	want := `cmd.exe /c C:\Users\test\AppData\Roaming\npm\opencode.cmd --model big-pickle`
	if got != want {
		t.Errorf("resolveConPtyCmdLine(.cmd shim):\n  got:  %s\n  want: %s", got, want)
	}
}

// TestResolveConPtyCmdLine_ExePassesThrough confirms .exe binaries (and any
// non-batch path) skip the cmd.exe wrapper — wrapping would lose access to
// signals and add a needless intermediate process for the common case.
func TestResolveConPtyCmdLine_ExePassesThrough(t *testing.T) {
	cmd := &exec.Cmd{
		Path: `C:\Windows\System32\cmd.exe`,
		Args: []string{"cmd.exe", "/c", "exit", "0"},
	}
	got := resolveConPtyCmdLine(cmd)
	if strings.HasPrefix(got, "cmd.exe /c cmd.exe") {
		t.Errorf("resolveConPtyCmdLine wrapped a non-batch executable: %s", got)
	}
}

// TestResolveConPtyCmdLine_PathSubstitutedForArgs0 confirms cmd.Path is what
// CreateProcessW receives, not the unresolved cmd.Args[0]. Without this,
// "opencode" reaches CreateProcessW and fails because the loader can't
// resolve a bare name without PATHEXT.
func TestResolveConPtyCmdLine_PathSubstitutedForArgs0(t *testing.T) {
	cmd := &exec.Cmd{
		Path: `C:\Tools\foo.exe`,
		Args: []string{"foo", "bar"},
	}
	got := resolveConPtyCmdLine(cmd)
	if !strings.HasPrefix(got, `C:\Tools\foo.exe `) {
		t.Errorf("expected cmd.Path as first token; got: %s", got)
	}
	if strings.HasPrefix(got, "foo ") {
		t.Errorf("first token is unresolved Args[0]; got: %s", got)
	}
}
