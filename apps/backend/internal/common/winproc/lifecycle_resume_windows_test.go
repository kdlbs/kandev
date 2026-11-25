package winproc

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func startSuspendedExit(t *testing.T, code string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("cmd.exe", "/d", "/c", "exit "+code)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_SUSPENDED}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start suspended: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return cmd
}

func waitExit(t *testing.T, cmd *exec.Cmd, want int) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var exitErr *exec.ExitError
		if err != nil && !errors.As(err, &exitErr) {
			t.Fatalf("wait: %v", err)
		}
		if got := cmd.ProcessState.ExitCode(); got != want {
			t.Fatalf("exit code = %d, want %d", got, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("process stayed suspended")
	}
}

// Every managed Git command on Windows starts suspended. Resuming it by process
// handle avoids a system-wide thread snapshot per command; the snapshot walk
// remains the fallback.
// @covers AC-EXECUTORS-SSH-WINDOWS-001.22
func TestResumeSuspendedProcessRunsItToCompletion(t *testing.T) {
	for name, resume := range map[string]func(int) error{
		"by handle":       resumeSuspendedProcessByHandle,
		"thread snapshot": resumeSuspendedProcessThreads,
	} {
		t.Run(name, func(t *testing.T) {
			cmd := startSuspendedExit(t, "7")
			if err := resume(cmd.Process.Pid); err != nil {
				t.Fatalf("resume: %v", err)
			}
			waitExit(t, cmd, 7)
		})
	}
}
