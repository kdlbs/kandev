package process

import (
	"os"
	"runtime"
	"testing"
)

func TestDefaultShellCommand_CustomShell(t *testing.T) {
	cmd := defaultShellCommand("/bin/sh")
	if cmd[0] != "/bin/sh" {
		t.Errorf("expected preferred shell as first element, got %q", cmd[0])
	}
}

func TestDefaultShellCommand_EmptyPreferred(t *testing.T) {
	cmd := defaultShellCommand("")
	if len(cmd) == 0 {
		t.Fatal("expected non-empty command")
	}
	// On Unix the shell must be invoked WITHOUT -l so /etc/profile doesn't
	// reset PATH (which would drop /data/.npm-global/bin where agent CLIs
	// installed via the Settings → Agents install button live).
	if runtime.GOOS != "windows" {
		for _, a := range cmd[1:] {
			if a == "-l" {
				t.Errorf("expected no -l flag on Unix shell, got %v", cmd)
			}
		}
	}
}

func TestDefaultShellCommand_InvalidPreferredFallsBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only fallback behavior")
	}
	originalShell := os.Getenv("SHELL")
	defer func() { _ = os.Setenv("SHELL", originalShell) }()

	_ = os.Setenv("SHELL", "/bin/sh")
	cmd := defaultShellCommand("/this/path/does/not/exist")
	if cmd[0] != "/bin/sh" {
		t.Fatalf("expected fallback to SHELL (/bin/sh), got %q", cmd[0])
	}
}

func TestShellExecArgs(t *testing.T) {
	prog, args := shellExecArgs("echo hello")
	if prog == "" {
		t.Fatal("expected non-empty program")
	}
	if len(args) == 0 {
		t.Fatal("expected non-empty args")
	}
	// The command string should appear in the args
	found := false
	for _, a := range args {
		if a == "echo hello" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected command string in args, got prog=%q args=%v", prog, args)
	}
}
