//go:build !windows

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

const abruptLossHelperEnv = "KANDEV_TEST_ABRUPT_LOSS_HELPER"
const abruptLossDescendantEnv = "KANDEV_TEST_ABRUPT_LOSS_DESCENDANT"
const abruptLossMarkerEnv = "KANDEV_TEST_ABRUPT_LOSS_MARKER"

// TestAbruptParentLossHelperProcess runs the production monitor and shutdown
// path inside the child, with a real owned descendant to clean up.
func TestAbruptParentLossHelperProcess(t *testing.T) {
	if os.Getenv(abruptLossHelperEnv) != "1" {
		return
	}
	marker := os.Getenv(abruptLossMarkerEnv)
	log := newTestLogger(t)

	descendant := exec.Command(os.Args[0], "-test.run=^TestAbruptParentLossDescendantProcess$")
	descendant.Env = append(os.Environ(), abruptLossDescendantEnv+"=1")
	if err := descendant.Start(); err != nil {
		_ = os.WriteFile(marker+".error", []byte(err.Error()), 0600)
		os.Exit(1)
	}
	readyTmp := marker + ".ready.tmp"
	if err := os.WriteFile(readyTmp, []byte(strconv.Itoa(descendant.Process.Pid)), 0600); err != nil {
		os.Exit(1)
	}
	if err := os.Rename(readyTmp, marker+".ready"); err != nil {
		os.Exit(1)
	}

	parentClosed := monitorParentLiveness(log)
	if parentClosed == nil {
		_ = os.WriteFile(marker+".error", []byte("parent liveness monitor was not armed"), 0600)
		os.Exit(1)
	}
	waitForShutdown(log, parentClosed, nil, func(context.Context) {
		_ = descendant.Process.Signal(syscall.SIGKILL)
		_ = descendant.Wait()
		_ = os.WriteFile(marker+".cleanup", []byte("cleanup"), 0600)
	})
	_ = os.WriteFile(marker+".shutdown", []byte("shutdown"), 0600)
	os.Exit(0)
}

// TestAbruptParentLossDescendantProcess is the owned descendant that the
// helper must reap after parent loss.
func TestAbruptParentLossDescendantProcess(t *testing.T) {
	if os.Getenv(abruptLossDescendantEnv) != "1" {
		return
	}
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

// @covers AC-PLATFORM-DIAGNOSTIC-SIGNAL-004.2
// @covers AC-PLATFORM-DIAGNOSTIC-SIGNAL-004.3
func TestAbruptParentLossCleansOwnedChild(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "marker")
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestAbruptParentLossHelperProcess$")
	cmd.Env = append(os.Environ(),
		abruptLossHelperEnv+"=1",
		abruptLossMarkerEnv+"="+marker,
		"KANDEV_PARENT_PIPE_FD=3",
	)
	cmd.ExtraFiles = []*os.File{readEnd}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = readEnd.Close()
	t.Cleanup(func() {
		_ = writeEnd.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})

	pid := waitForAbruptLossReady(t, marker)
	t.Cleanup(func() {
		if err := syscall.Kill(pid, 0); err == nil {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	if err := writeEnd.Close(); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("helper did not exit cleanly after parent loss: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("helper did not exit after parent-liveness pipe closure")
	}
	for _, suffix := range []string{".cleanup", ".shutdown"} {
		if _, err := os.Stat(marker + suffix); err != nil {
			t.Fatalf("helper did not complete %s: %v", suffix, err)
		}
	}
	if err := syscall.Kill(pid, 0); !isProcessGoneError(err) {
		t.Fatalf("owned descendant %d still exists after cleanup: %v", pid, err)
	}
}

func waitForAbruptLossReady(t *testing.T, marker string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		content, err := os.ReadFile(marker + ".ready")
		if err == nil {
			pid, parseErr := strconv.Atoi(string(content))
			if parseErr != nil {
				t.Fatalf("invalid descendant pid %q: %v", content, parseErr)
			}
			return pid
		}
		if _, statErr := os.Stat(marker + ".error"); statErr == nil {
			message, _ := os.ReadFile(marker + ".error")
			t.Fatalf("helper failed before readiness: %s", message)
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not report readiness")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func isProcessGoneError(err error) bool {
	return errors.Is(err, syscall.ESRCH)
}
