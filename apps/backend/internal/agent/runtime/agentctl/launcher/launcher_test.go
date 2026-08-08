package launcher

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestLauncherConfigSupportsUnexpectedExitCallback(t *testing.T) {
	field, ok := reflect.TypeOf(Config{}).FieldByName("OnUnexpectedExit")
	if !ok {
		t.Fatal("launcher config has no unexpected-exit callback")
	}
	if field.Type != reflect.TypeOf((func())(nil)) {
		t.Fatalf("unexpected-exit callback type = %v, want func()", field.Type)
	}
}

func TestLauncherUnexpectedExitCallbackRunsForCleanAndFailedExit(t *testing.T) {
	for _, exitCode := range []int{0, 7} {
		t.Run(fmt.Sprintf("exit-%d", exitCode), func(t *testing.T) {
			called := startLauncherMonitorTest(t, exitCode, false)
			if !called {
				t.Fatalf("unexpected-exit callback was not called for exit code %d", exitCode)
			}
		})
	}
}

func TestLauncherUnexpectedExitCallbackSkipsIntentionalStop(t *testing.T) {
	if called := startLauncherMonitorTest(t, 7, true); called {
		t.Fatal("unexpected-exit callback ran during intentional stop")
	}
}

func startLauncherMonitorTest(t *testing.T, exitCode int, stopping bool) bool {
	t.Helper()
	cmd := launcherExitCommand(exitCode)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start launcher child: %v", err)
	}

	callback := make(chan struct{}, 1)
	launcher := &Launcher{
		cmd:              cmd,
		exited:           make(chan struct{}),
		logger:           newUnexpectedExitTestLogger(t),
		onUnexpectedExit: func() { callback <- struct{}{} },
	}
	launcher.mu.Lock()
	launcher.stopping = stopping
	launcher.mu.Unlock()
	go launcher.monitorExit()

	select {
	case <-launcher.exited:
	case <-time.After(time.Second):
		t.Fatal("launcher monitor did not finish")
	}
	select {
	case <-callback:
		return true
	default:
		return false
	}
}

func launcherExitCommand(exitCode int) *exec.Cmd {
	code := strconv.Itoa(exitCode)
	if runtime.GOOS == "windows" {
		return exec.Command("cmd.exe", "/c", "exit", code)
	}
	return exec.Command("sh", "-c", "exit "+code)
}

func newUnexpectedExitTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return log
}

func TestCloseParentPipe_ClosesAndNils(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer func() { _ = r.Close() }()

	l := &Launcher{parentPipe: w}
	l.closeParentPipe()

	if l.parentPipe != nil {
		t.Error("parentPipe should be nil after closeParentPipe")
	}

	// Writing to the closed write-end should fail.
	_, err = w.Write([]byte("x"))
	if err == nil {
		t.Error("expected error writing to closed pipe")
	}
}

func TestCloseParentPipe_NilIsNoop(t *testing.T) {
	l := &Launcher{}
	// Must not panic.
	l.closeParentPipe()
	if l.parentPipe != nil {
		t.Error("parentPipe should remain nil")
	}
}

func TestCloseParentPipe_Idempotent(t *testing.T) {
	_, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	l := &Launcher{parentPipe: w}
	l.closeParentPipe()
	// Second call must not panic (double close).
	l.closeParentPipe()
}
