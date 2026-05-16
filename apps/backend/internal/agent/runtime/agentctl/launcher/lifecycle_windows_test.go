//go:build windows

package launcher

import (
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"golang.org/x/sys/windows"
)

func newTestLauncher(t *testing.T) *Launcher {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return &Launcher{logger: log}
}

func TestCreateKillOnCloseJob(t *testing.T) {
	job, err := createKillOnCloseJob()
	if err != nil {
		t.Fatalf("createKillOnCloseJob: %v", err)
	}
	if job == 0 {
		t.Fatal("got null job handle")
	}
	if err := windows.CloseHandle(job); err != nil {
		t.Errorf("CloseHandle: %v", err)
	}
}

func TestReleaseChildLifecycle_NoHandleIsNoop(t *testing.T) {
	l := newTestLauncher(t)
	// Must not panic when no handle has been installed.
	l.releaseChildLifecycle()
	if atomic.LoadUintptr(&l.jobHandle) != 0 {
		t.Error("jobHandle should remain zero")
	}
}

func TestReleaseChildLifecycle_Idempotent(t *testing.T) {
	job, err := createKillOnCloseJob()
	if err != nil {
		t.Fatalf("createKillOnCloseJob: %v", err)
	}

	l := newTestLauncher(t)
	atomic.StoreUintptr(&l.jobHandle, uintptr(job))

	l.releaseChildLifecycle()
	// Second call must not double-close the handle.
	l.releaseChildLifecycle()
	if atomic.LoadUintptr(&l.jobHandle) != 0 {
		t.Error("jobHandle should be cleared after release")
	}
}

// TestInstallChildLifecycle_KillOnHandleClose verifies the core invariant: a
// process bound to the job is terminated by the OS the moment the last handle
// to the job is closed. This is what protects against orphaned agentctl.exe
// when the backend crashes (issue #892) — Windows closes our handles for us
// as part of process teardown.
func TestInstallChildLifecycle_KillOnHandleClose(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "ping", "-n", "30", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	defer func() {
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			_ = cmd.Process.Kill()
		}
	}()

	l := newTestLauncher(t)
	if err := l.installChildLifecycle(cmd); err != nil {
		t.Fatalf("installChildLifecycle: %v", err)
	}
	if atomic.LoadUintptr(&l.jobHandle) == 0 {
		t.Fatal("jobHandle was not stored")
	}

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	// Closing the only handle to the job must kill the assigned process.
	l.releaseChildLifecycle()

	select {
	case <-exited:
		// Either err or nil exit, both fine — what matters is that the
		// process did not survive the handle close.
	case <-time.After(5 * time.Second):
		t.Fatal("child still alive 5s after job-handle close")
	}
}
