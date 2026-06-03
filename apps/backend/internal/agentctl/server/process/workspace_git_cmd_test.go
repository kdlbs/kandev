//go:build !windows

package process

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
)

// TestRunGitOutput_ReleasesThrottleOnTimeout is the regression test for the
// "empty Changes panel after freeze" symptom (see workspace_monitor.go
// gitCommandTimeout). It proves that when a git invocation wedges past
// gitCommandTimeout, runGitOutput SIGKILLs the subprocess and releases its
// gitThrottle slot so queued callers can still make progress.
//
// Mechanism: install a 'git' shim on $PATH that sleeps for an hour, shrink
// gitCommandTimeout to 200ms and the throttle cap to 2, then spawn 6
// goroutines all calling runGitOutput. Without the per-command timeout the
// 4 queued goroutines would block on Acquire forever (the test would hang
// past its bound). With the fix each batch of 2 times out, releases, and
// the next pair Acquires — total ≈ 200ms × 3 batches = 600ms.
func TestRunGitOutput_ReleasesThrottleOnTimeout(t *testing.T) {
	prev := gitCommandTimeout
	gitCommandTimeout = 200 * time.Millisecond
	t.Cleanup(func() { gitCommandTimeout = prev })

	restoreCap := subproc.Git().SetCapForTest(2)
	t.Cleanup(restoreCap)

	shimDir := installSleepGitShim(t)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, wt := setupTestDir(t)

	const N = 6
	var wg sync.WaitGroup
	wg.Add(N)
	done := make(chan struct{})
	start := time.Now()
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, _ = wt.runGitOutput(context.Background(), "status")
		}()
	}
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
		elapsed := time.Since(start)
		// 6 calls through cap=2 with ~200ms each ⇒ ~600ms. 5s is a generous
		// upper bound that still catches a leaked slot (which would hang
		// indefinitely and only be observed via the select timeout below).
		if elapsed > 5*time.Second {
			t.Fatalf("runGitOutput took %v for %d calls through cap=2; throttle slot may have leaked", elapsed, N)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runGitOutput hung past 5s — throttle slot leaked on per-command timeout")
	}
}

// TestRunGitOutput_PollingVariantReleasesThrottleOnTimeout mirrors the
// regression above for runPollingGitOutput, which is the entry point used
// by the actual workspace poll loop. Same setup, same expectations.
func TestRunGitOutput_PollingVariantReleasesThrottleOnTimeout(t *testing.T) {
	prev := gitCommandTimeout
	gitCommandTimeout = 200 * time.Millisecond
	t.Cleanup(func() { gitCommandTimeout = prev })

	restoreCap := subproc.Git().SetCapForTest(2)
	t.Cleanup(restoreCap)

	shimDir := installSleepGitShim(t)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, wt := setupTestDir(t)

	const N = 6
	var wg sync.WaitGroup
	wg.Add(N)
	done := make(chan struct{})
	start := time.Now()
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, _ = wt.runPollingGitOutput(context.Background(), "status")
		}()
	}
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("runPollingGitOutput took %v for %d calls through cap=2; throttle slot may have leaked", elapsed, N)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runPollingGitOutput hung past 5s — throttle slot leaked on per-command timeout")
	}
}

// installSleepGitShim writes an executable shell script named 'git' into a
// fresh tmpdir that blocks for an hour, and returns the dir. Callers must
// prepend it to $PATH so exec.Command("git", ...) picks it up before the
// real binary. The long sleep guarantees that any successful completion
// must come from the per-command timeout SIGKILLing the subprocess.
//
// `exec sleep 3600` (instead of `sleep 3600`) makes sleep replace sh as
// the process image, so SIGKILL on the exec.Cmd's pid lands directly on
// sleep. Otherwise sh would die, leave sleep orphaned still holding the
// stdout/stderr pipes, and cmd.Output() would block waiting for pipe
// closure for the full hour — making the test flaky.
func installSleepGitShim(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	shim := filepath.Join(dir, "git")
	const script = "#!/bin/sh\nexec sleep 3600\n"
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	return dir
}
