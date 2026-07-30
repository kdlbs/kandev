//go:build linux || darwin

package gitinit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandContextInitializesInheritedDirectory(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "staging")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir staging: %v", err)
	}
	directory, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open staging: %v", err)
	}
	t.Cleanup(func() { _ = directory.Close() })

	openedPath := filepath.Join(parent, "opened")
	if err := os.Rename(path, openedPath); err != nil {
		t.Fatalf("Rename staging: %v", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir replacement: %v", err)
	}

	command, err := CommandContext(context.Background(), path, directory)
	if err != nil {
		t.Fatalf("CommandContext: %v", err)
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}

	if info, err := os.Stat(filepath.Join(openedPath, ".git")); err != nil || !info.IsDir() {
		t.Fatalf("opened directory .git: info=%v error=%v", info, err)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("ReadDir replacement: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("replacement entries = %+v, want none", entries)
	}
}

func TestCommandContextRejectsMissingGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	directory, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open directory: %v", err)
	}
	t.Cleanup(func() { _ = directory.Close() })

	command, err := CommandContext(context.Background(), directory.Name(), directory)
	if err == nil {
		t.Fatalf("CommandContext command = %v, want missing Git error", command)
	}
}

func TestRunHelperRejectsNonDirectoryDescriptor(t *testing.T) {
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	t.Cleanup(func() { _ = readPipe.Close() })
	t.Cleanup(func() { _ = writePipe.Close() })

	command := exec.Command(os.Args[0], helperArgument, os.Args[0])
	command.ExtraFiles = []*os.File{readPipe}
	command.Env = withHelperEnvironment(os.Environ())
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("helper succeeded with a pipe as fd 3")
	}
	if !strings.Contains(string(output), "enter inherited directory") {
		t.Fatalf("helper output = %q, want inherited directory error", output)
	}
}
