package mcpconfig

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveExecutableUsesRuntimePathOverride(t *testing.T) {
	directory := t.TempDir()
	name := "cursor-agent"
	if runtime.GOOS == "windows" {
		name += ".EXE"
	}
	executable := filepath.Join(directory, name)
	if err := os.WriteFile(executable, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveExecutable("cursor-agent", map[string]string{"PATH": directory})
	if err != nil {
		t.Fatalf("resolve executable from runtime PATH: %v", err)
	}
	if resolved != executable {
		t.Fatalf("resolved executable = %q, want %q", resolved, executable)
	}
}

func TestResolveExecutableHonorsHostExecutableRules(t *testing.T) {
	directory := t.TempDir()
	name := "cursor-agent"
	if runtime.GOOS == "windows" {
		name += ".EXE"
	}
	executable := filepath.Join(directory, name)
	if err := os.WriteFile(executable, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveExecutable("cursor-agent", map[string]string{"PATH": directory})
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("Windows executable should not require Unix execute bits: %v", err)
		}
		if resolved != executable {
			t.Fatalf("resolved executable = %q, want %q", resolved, executable)
		}
		return
	}
	if !errors.Is(err, ErrNativeMCPExecutableUnavailable) {
		t.Fatalf("non-executable Unix file error = %v, want unavailable", err)
	}
	if err := os.Chmod(executable, 0o700); err != nil {
		t.Fatal(err)
	}
	resolved, err = resolveExecutable("cursor-agent", map[string]string{"PATH": directory})
	if err != nil {
		t.Fatalf("resolve executable after adding execute permission: %v", err)
	}
	if resolved != executable {
		t.Fatalf("resolved executable = %q, want %q", resolved, executable)
	}
}

func TestResolveExecutableTreatsExplicitEmptyPathAsOverride(t *testing.T) {
	command, _ := hostShellCommand(t)
	_, err := resolveExecutable(filepath.Base(command), map[string]string{"PATH": ""})
	if !errors.Is(err, ErrNativeMCPExecutableUnavailable) {
		t.Fatalf("resolve executable with explicitly empty PATH error = %v, want unavailable", err)
	}
}

func TestExecNativeMCPCommandRunnerPropagatesStartFailure(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "missing-workspace")
	command, args := hostShellCommand(t)
	_, err := (ExecNativeMCPCommandRunner{}).Run(context.Background(), command, args, workspace, nil)
	if !errors.Is(err, ErrNativeMCPExecutableUnavailable) {
		t.Fatalf("runner start failure = %v, want executable unavailable", err)
	}
}

func hostShellCommand(t *testing.T) (string, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/c", "exit", "0"}
	}
	return "/bin/sh", []string{"-c", "exit 0"}
}
