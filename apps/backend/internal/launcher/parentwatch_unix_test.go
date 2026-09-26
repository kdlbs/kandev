//go:build !windows

package launcher

import (
	"os/exec"
	"testing"
	"time"
)

func TestParentWatchdogDetectsReapedProcess(t *testing.T) {
	child := exec.Command("sleep", "10")
	if err := child.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	t.Cleanup(func() {
		if child.ProcessState == nil {
			_ = child.Process.Kill()
		}
		_ = child.Wait()
	})

	shutdown := make(chan string, 1)
	exited := make(chan int, 1)
	watchdog := newParentWatchdog(int64(child.Process.Pid), func(reason string) {
		shutdown <- reason
	}, func(code int) {
		exited <- code
	})
	watchdog.interval = 5 * time.Millisecond
	watchdog.start()
	t.Cleanup(watchdog.stop)

	if err := child.Process.Kill(); err != nil {
		t.Fatalf("kill child: %v", err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("child exited successfully after kill")
	}

	select {
	case reason := <-shutdown:
		if reason != "parent process exit" {
			t.Fatalf("shutdown reason = %q, want parent process exit", reason)
		}
	case <-time.After(time.Second):
		t.Fatal("watchdog did not detect the reaped process")
	}
	select {
	case code := <-exited:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	case <-time.After(time.Second):
		t.Fatal("watchdog did not request launcher exit")
	}
}
