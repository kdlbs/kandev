package shellexec

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestResolve_Unix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only resolution")
	}
	t.Run("sh hint", func(t *testing.T) {
		prog, args := resolve(PosixSh, "echo hi")
		if prog != "sh" {
			t.Errorf("prog = %q, want sh", prog)
		}
		if len(args) != 2 || args[0] != "-c" || args[1] != "echo hi" {
			t.Errorf("args = %v, want [-c echo hi]", args)
		}
	})
	t.Run("bash hint", func(t *testing.T) {
		prog, args := resolve(Bash, "echo hi")
		if prog != "bash" {
			t.Errorf("prog = %q, want bash", prog)
		}
		if len(args) != 2 || args[0] != "-c" || args[1] != "echo hi" {
			t.Errorf("args = %v, want [-c echo hi]", args)
		}
	})
}

func TestResolve_Windows_PrefersBash(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only resolution")
	}
	original := findWindowsBash
	t.Cleanup(func() { findWindowsBash = original })
	findWindowsBash = func() string { return `C:\fake\bash.exe` }

	prog, args := resolve(PosixSh, "echo hi")
	if prog != `C:\fake\bash.exe` {
		t.Errorf("prog = %q, want fake bash path", prog)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "echo hi" {
		t.Errorf("args = %v", args)
	}
}

func TestResolve_Windows_FallsBackToCmd(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only resolution")
	}
	original := findWindowsBash
	t.Cleanup(func() { findWindowsBash = original })
	findWindowsBash = func() string { return "" }

	prog, args := resolve(PosixSh, "echo hi")
	if prog != "cmd" {
		t.Errorf("prog = %q, want cmd", prog)
	}
	if len(args) != 2 || args[0] != "/c" || args[1] != "echo hi" {
		t.Errorf("args = %v", args)
	}
}

// TestCommandContext_RunsScript end-to-end — picks whatever the host has and
// confirms the script actually runs. Uses a portable `echo` that works in
// every supported shell (sh, bash, cmd).
func TestCommandContext_RunsScript(t *testing.T) {
	cmd := CommandContext(context.Background(), PosixSh, "echo hello-shellexec")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v (output: %s)", err, out)
	}
	if !strings.Contains(string(out), "hello-shellexec") {
		t.Errorf("output = %q, want substring hello-shellexec", out)
	}
}
