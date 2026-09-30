//go:build windows

package subproc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsPrepareGitLifecycleCommandRequestsNoConsoleWindow(t *testing.T) {
	cmd := exec.Command("git", "status")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.BELOW_NORMAL_PRIORITY_CLASS}
	if err := prepareGitLifecycleCommand(cmd); err != nil {
		t.Fatalf("prepareGitLifecycleCommand: %v", err)
	}
	flags := cmd.SysProcAttr.CreationFlags
	for _, want := range []uint32{
		windows.BELOW_NORMAL_PRIORITY_CLASS,
		syscall.CREATE_NEW_PROCESS_GROUP,
		windows.CREATE_SUSPENDED,
		windows.CREATE_NO_WINDOW,
	} {
		if flags&want == 0 {
			t.Fatalf("CreationFlags = %#x, want %#x set", flags, want)
		}
	}

	console := exec.Command("git", "status")
	console.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
	if err := prepareGitLifecycleCommand(console); err == nil {
		t.Fatal("prepareGitLifecycleCommand accepted CREATE_NEW_CONSOLE")
	}
}

func TestWindowsManagedGitJobCleanup(t *testing.T) {
	dir := t.TempDir()
	started := filepath.Join(dir, "started")
	gitCommand := filepath.Join(dir, "git.cmd")
	if err := os.WriteFile(gitCommand, []byte("@echo off\r\nif \"%1\"==\"hold\" (\r\n  echo started>\"%GIT_TEST_STARTED%\"\r\n  cmd.exe /c \"ping.exe 127.0.0.1 -n 100 >nul\"\r\n) else (\r\n  echo quick\r\n)\r\n"), 0o700); err != nil {
		t.Fatalf("write Git fixture: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GIT_TEST_STARTED", started)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, runErr, execErr := RunGitOutputAfterAcquire(ctx, GitLifecycle, 5*time.Second, func(execCtx context.Context) *exec.Cmd {
			return NewGitCommand(execCtx, "hold")
		})
		if runErr != nil {
			result <- runErr
			return
		}
		result <- execErr
	}()
	waitForWindowsGitFile(t, started, time.Second)
	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("canceled managed Git command unexpectedly succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("managed Git Job Object did not terminate the owned process tree")
	}
}

func waitForWindowsGitFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", path)
		}
	}
}
