package process

import (
	"runtime"
	"testing"
)

func TestDefaultShellCommand_CustomShell(t *testing.T) {
	cmd := defaultShellCommand("/usr/local/bin/fish")
	if cmd[0] != "/usr/local/bin/fish" {
		t.Errorf("expected preferred shell as first element, got %q", cmd[0])
	}
}

func TestDefaultShellCommand_EmptyPreferred(t *testing.T) {
	cmd := defaultShellCommand("")
	if len(cmd) == 0 {
		t.Fatal("expected non-empty command")
	}
	// On Unix, should fall back to $SHELL or /bin/sh with -l flag
	// On Windows, should fall back to %COMSPEC% or powershell.exe
	if runtime.GOOS != "windows" {
		if cmd[len(cmd)-1] != "-l" {
			t.Errorf("expected login flag -l on Unix, got %v", cmd)
		}
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
