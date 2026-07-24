package worktree

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
)

// TestRunGitCmd_SaturatedThrottleBlocks verifies that runGitCmd routes
// through the package-wide gitThrottle: once the cap is saturated, the
// next call must block until a holder releases. This is the host-freeze
// mitigation in action — without the throttle, lifecycle bursts
// (worktree add + branch -D + worktree prune + submodule update) for
// several agents starting in parallel could spawn 20+ simultaneous git
// processes, saturating the macOS code-signing fork queue.
//
// Uses gitThrottle.Acquire directly to saturate the pool (no real
// subprocess needed for the saturation half), then `sh -c true` to
// confirm runGitCmd itself goes through the same gate.
func TestRunGitCmd_SaturatedThrottleBlocks(t *testing.T) {
	const cap = 2
	restore := setGitThrottleCapForTest(cap)
	defer restore()

	// Saturate the pool out-of-band so the next runGitCmd is forced to
	// wait. Holding the slots via gitThrottle.Acquire (the same pool
	// runGitCmd uses) is the cleanest way to set this up without timing
	// against a real subprocess. Capture each release so the slots are
	// returned to the pool when the test ends — keeps the package-wide
	// gitThrottle clean for any subsequent test in the same run.
	for i := 0; i < cap; i++ {
		release, err := subproc.Git().Acquire(context.Background())
		if err != nil {
			t.Fatalf("pre-saturate acquire %d: %v", i, err)
		}
		defer release()
	}

	// A runGitCmd call with a short-deadline ctx must surface the
	// throttle's ctx.Err — proving it tried to acquire and got blocked
	// rather than exec'ing the subprocess and racing past the throttle.
	cctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := runGitCmd(cctx, exec.Command("sh", "-c", "true"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded once cap=%d saturated, got %v", cap, err)
	}
}

// TestRunGitCmd_RunsToCompletionWhenCapacityAvailable is the positive
// sibling: with capacity free, runGitCmd executes the command and
// returns its result. Guards against a regression where a refactor
// would acquire and immediately error out without running the command.
func TestRunGitCmd_RunsToCompletionWhenCapacityAvailable(t *testing.T) {
	restore := setGitThrottleCapForTest(4)
	defer restore()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runGitCmd(context.Background(), exec.Command("sh", "-c", "true")); err != nil {
				t.Errorf("runGitCmd unexpected err: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestNewNonInteractiveGitCmd_EnablesLongPathsPerCommand(t *testing.T) {
	cmd := (&Manager{}).newNonInteractiveGitCmd(context.Background(), t.TempDir(), "status", "--short")

	want := []string{"git", "-c", "core.longpaths=true", "status", "--short"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("git command args = %q, want %q", cmd.Args, want)
	}
}

// Cap parsing tests live in internal/common/subproc/shared_test.go now
// that the env var, default, and resolver are owned by that package.
