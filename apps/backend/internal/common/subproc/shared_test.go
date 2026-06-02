package subproc

import (
	"context"
	"os/exec"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestResolveCap(t *testing.T) {
	const env = "KANDEV_SUBPROC_CAP_TEST"
	const def = 7

	cases := []struct {
		name string
		val  string
		want int
	}{
		{"empty", "", def},
		{"valid", "3", 3},
		{"zero falls back", "0", def},
		{"negative falls back", "-1", def},
		{"garbage falls back", "abc", def},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(env, tc.val)
			if got := resolveCap(env, def); got != tc.want {
				t.Errorf("resolveCap(%q, %d) = %d, want %d", tc.val, def, got, tc.want)
			}
		})
	}
}

// TestResolveGHMaxConcurrent and TestResolveGitMaxConcurrent guard the
// process-wide defaults: a typo'd env or a fall-through must land on
// the safe default rather than disable the throttle.
func TestResolveGHMaxConcurrent(t *testing.T) {
	t.Setenv(ghMaxConcurrentEnv, "")
	if got := resolveGHMaxConcurrent(); got != defaultGHMaxConcurrent {
		t.Errorf("resolveGHMaxConcurrent() = %d, want %d", got, defaultGHMaxConcurrent)
	}
	t.Setenv(ghMaxConcurrentEnv, "garbage")
	if got := resolveGHMaxConcurrent(); got != defaultGHMaxConcurrent {
		t.Errorf("garbage env: got %d, want %d", got, defaultGHMaxConcurrent)
	}
	t.Setenv(ghMaxConcurrentEnv, "5")
	if got := resolveGHMaxConcurrent(); got != 5 {
		t.Errorf("valid env: got %d, want 5", got)
	}
}

func TestResolveGitMaxConcurrent(t *testing.T) {
	t.Setenv(gitMaxConcurrentEnv, "")
	if got := resolveGitMaxConcurrent(); got != defaultGitMaxConcurrent {
		t.Errorf("resolveGitMaxConcurrent() = %d, want %d", got, defaultGitMaxConcurrent)
	}
	t.Setenv(gitMaxConcurrentEnv, "0")
	if got := resolveGitMaxConcurrent(); got != defaultGitMaxConcurrent {
		t.Errorf("zero env: got %d, want %d", got, defaultGitMaxConcurrent)
	}
	t.Setenv(gitMaxConcurrentEnv, "20")
	if got := resolveGitMaxConcurrent(); got != 20 {
		t.Errorf("valid env: got %d, want 20", got)
	}
}

// TestGHGitAreDistinctThrottles verifies the two singletons aren't
// accidentally aliased. A regression where both helpers return the same
// pool would let a gh storm starve git ops (or vice-versa) — exactly
// the cross-contention the per-binary split is meant to prevent.
func TestGHGitAreDistinctThrottles(t *testing.T) {
	if GH() == Git() {
		t.Fatal("GH() and Git() returned the same throttle instance")
	}
}

// TestRunGHCombinedAfterAcquire_ExecTimeoutSurvivesAcquireWait is the
// regression test for the killed (192×) / context-deadline-exceeded
// (96×) cascade in the SyncWatchesBatched storm: when a queued waiter
// finally gets a throttle slot, its per-command exec budget MUST start
// fresh from that moment, not be eaten by the queue wait.
//
// Setup: pin the gh throttle to cap=1 and hold the slot for the
// acquireWait window. The second caller invokes
// RunGHCombinedAfterAcquire with a build closure that just records the
// execCtx deadline relative to the current wallclock. Pre-fix (timer
// started before Acquire) the recorded deadline would be in the past
// the moment Acquire returns; post-fix the deadline is acquireWait
// ahead of the AfterFunc fire, i.e. ~execTimeout ahead of now.
func TestRunGHCombinedAfterAcquire_ExecTimeoutSurvivesAcquireWait(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX `true` binary as a no-op subprocess")
	}
	restore := GH().SetCapForTest(1)
	defer restore()

	ctx := context.Background()
	holdRelease, err := GH().Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire holder: %v", err)
	}

	// Sized so a pre-fix regression (timer starts BEFORE acquire) yields
	// a NEGATIVE remaining budget at the build callback, while a correct
	// after-acquire timer yields a budget within (~50%, 100%] of
	// execTimeout. The exact fraction depends on scheduler latency
	// between AfterFunc firing and the second goroutine resuming, so the
	// lower bound is generous.
	const acquireWait = 200 * time.Millisecond
	const execTimeout = 500 * time.Millisecond

	var wg sync.WaitGroup
	wg.Add(1)
	var (
		gotRemaining time.Duration
		gotRunErr    error
	)
	go func() {
		defer wg.Done()
		_, gotRunErr, _ = RunGHCombinedAfterAcquire(ctx, execTimeout, func(execCtx context.Context) *exec.Cmd {
			if dl, ok := execCtx.Deadline(); ok {
				gotRemaining = time.Until(dl)
			}
			return exec.CommandContext(execCtx, "true")
		})
	}()
	time.AfterFunc(acquireWait, holdRelease)
	wg.Wait()

	// Pre-fix: gotRemaining would be roughly execTimeout - acquireWait,
	// or negative if acquireWait > execTimeout. Post-fix: gotRemaining
	// should land in (execTimeout/2, execTimeout].
	if gotRemaining <= execTimeout/2 {
		t.Errorf("execCtx deadline was only %v from now (execTimeout=%v); acquire wait burned through the budget", gotRemaining, execTimeout)
	}
	if gotRunErr != nil {
		t.Logf("`true` runErr=%v (informational; deadline budget is the assertion)", gotRunErr)
	}
}
