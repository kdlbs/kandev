package subproc

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// defaultGHExecTimeout is the per-command exec budget the
// Run*AfterAcquire helpers apply after the throttle slot is acquired.
// 30s matches the gh client's default WithTimeout for plain `gh api`
// calls; callers needing more (paginated REST calls) or less (cheap
// `gh auth status`) pass an explicit value.
const defaultGHExecTimeout = 30 * time.Second

// Shared throttle singletons.
//
// Both gh and git are spawned from many packages across the backend
// (PR poller, worktree manager, agentctl process group, agent lifecycle
// credential uploader, repoclone, ...). To make the cap actually global
// across the process, the singleton lives here — at the lowest layer
// any of those callers already depend on — instead of inside one of the
// higher-level packages. That way no caller has to import a sibling
// package solely to share its semaphore.

const (
	// defaultGHMaxConcurrent and defaultGitMaxConcurrent are sized to
	// stay below the spawn rate at which macOS code-signing + EDR
	// latency (CrowdStrike Falcon + syspolicyd) starts to back up and
	// freeze the host. Git's cap is higher than gh's because typical
	// git work is local-only and drains the queue faster.
	defaultGHMaxConcurrent  = 8
	defaultGitMaxConcurrent = 12

	ghMaxConcurrentEnv  = "KANDEV_GH_MAX_CONCURRENT"
	gitMaxConcurrentEnv = "KANDEV_GIT_MAX_CONCURRENT"
)

var (
	// Names ("gh", "git") double as the expvar.Map keys under
	// /debug/vars (subproc_cap, subproc_inflight, subproc_waiters,
	// subproc_acquire_total, subproc_acquire_wait_millis_total). Unit
	// tests that swap the pool via SetCapForTest reuse the same names
	// so the published gauges stay coherent across cap changes.
	ghThrottle  = NewNamedThrottle("gh", resolveCap(ghMaxConcurrentEnv, defaultGHMaxConcurrent))
	gitThrottle = NewNamedClassThrottle("git", resolveCap(gitMaxConcurrentEnv, defaultGitMaxConcurrent))
)

// GH returns the process-wide throttle gating gh subprocess execs.
// All gh callers across the codebase share this single semaphore so the
// total host fork pressure stays bounded regardless of caller count.
func GH() *Throttle { return ghThrottle }

// Git returns the process-wide throttle gating git subprocess execs.
// Shared by the worktree manager, agentctl process group, agent runtime
// env preparers, and any other call site that shells out to git.
func Git() *Throttle { return gitThrottle }

// resolveCap reads env for an integer cap, falling back to def for
// missing/invalid/non-positive values. cap parsing is intentionally
// permissive in only the failure direction — typos and clears revert
// to the safe default rather than disabling the throttle silently.
func resolveCap(env string, def int) int {
	raw := os.Getenv(env)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// resolveGHMaxConcurrent and resolveGitMaxConcurrent are kept as
// package-private accessors so the unit tests can verify env parsing
// without exporting the parser itself. Production code constructs the
// singleton at init time and never re-reads the env.
func resolveGHMaxConcurrent() int  { return resolveCap(ghMaxConcurrentEnv, defaultGHMaxConcurrent) }
func resolveGitMaxConcurrent() int { return resolveCap(gitMaxConcurrentEnv, defaultGitMaxConcurrent) }

// RunGitClass acquires a Git slot in class, runs cmd, and releases the slot.
// The caller owns cmd.Stdout/Stderr wiring.
func RunGitClass(ctx context.Context, class GitWorkClass, cmd *exec.Cmd) error {
	release, err := gitThrottle.AcquireClass(ctx, class)
	if err != nil {
		return err
	}
	defer release()
	return cmd.Run()
}

// RunGitCombinedOutputClass is RunGitClass's CombinedOutput sibling.
func RunGitCombinedOutputClass(ctx context.Context, class GitWorkClass, cmd *exec.Cmd) ([]byte, error) {
	release, err := gitThrottle.AcquireClass(ctx, class)
	if err != nil {
		return nil, err
	}
	defer release()
	return cmd.CombinedOutput()
}

// RunGitOutputClass is RunGitClass's Output sibling. Stderr is captured in
// (*exec.ExitError).Stderr only when cmd.Stderr is left nil; callers that set
// cmd.Stderr must read it from their provided writer.
func RunGitOutputClass(ctx context.Context, class GitWorkClass, cmd *exec.Cmd) ([]byte, error) {
	release, err := gitThrottle.AcquireClass(ctx, class)
	if err != nil {
		return nil, err
	}
	defer release()
	return cmd.Output()
}

// RunGitCombinedAfterAcquire starts the execution timeout only after the
// requested class receives a Git slot. The builder runs inside the slot so
// command construction cannot consume the execution budget while queued.
func RunGitCombinedAfterAcquire(
	ctx context.Context,
	class GitWorkClass,
	execTimeout time.Duration,
	build func(execCtx context.Context) *exec.Cmd,
) ([]byte, error, error) {
	release, err := gitThrottle.AcquireClass(ctx, class)
	if err != nil {
		return nil, wrapAdmissionError(err), nil
	}
	defer release()
	execCtx, cancel := withExecTimeout(ctx, execTimeout)
	defer cancel()
	out, runErr := build(execCtx).CombinedOutput()
	return out, runErr, execCtx.Err()
}

// RunGitOutputAfterAcquire is RunGitCombinedAfterAcquire's Output sibling.
// The command is built only after admission, and stderr retains exec.Cmd's
// standard Output behavior through *exec.ExitError.Stderr. The acquisition
// context also owns the execution deadline; use
// RunGitOutputAfterAcquireWithExecutionContext when those lifetimes differ.
func RunGitOutputAfterAcquire(
	ctx context.Context,
	class GitWorkClass,
	execTimeout time.Duration,
	build func(execCtx context.Context) *exec.Cmd,
) ([]byte, error, error) {
	return RunGitOutputAfterAcquireWithExecutionContext(ctx, ctx, class, execTimeout, build)
}

// RunGitOutputAfterAcquireWithExecutionContext admits using acquireCtx, then
// starts the command timeout from execBaseCtx only after a slot is granted.
// This is useful for best-effort probes that need a bounded queue wait but
// must not inherit the queue deadline as their execution deadline.
func RunGitOutputAfterAcquireWithExecutionContext(
	acquireCtx context.Context,
	execBaseCtx context.Context,
	class GitWorkClass,
	execTimeout time.Duration,
	build func(execCtx context.Context) *exec.Cmd,
) ([]byte, error, error) {
	release, err := gitThrottle.AcquireClass(acquireCtx, class)
	if err != nil {
		return nil, wrapAdmissionError(err), nil
	}
	defer release()
	execCtx, cancel := withExecTimeout(execBaseCtx, execTimeout)
	defer cancel()
	out, runErr := build(execCtx).Output()
	return out, runErr, execCtx.Err()
}

// RunGitAfterAcquire is RunGitCombinedAfterAcquire's plain Run sibling.
func RunGitAfterAcquire(
	ctx context.Context,
	class GitWorkClass,
	execTimeout time.Duration,
	build func(execCtx context.Context) *exec.Cmd,
) (error, error) {
	release, err := gitThrottle.AcquireClass(ctx, class)
	if err != nil {
		return wrapAdmissionError(err), nil
	}
	defer release()
	execCtx, cancel := withExecTimeout(ctx, execTimeout)
	defer cancel()
	runErr := build(execCtx).Run()
	return runErr, execCtx.Err()
}

// RunGH / RunGHOutput / RunGHCombinedOutput mirror the git helpers but
// gate on the gh throttle. Keep these in sync with the git triplet —
// if a new exec method is needed (e.g. StdoutPipe streaming), add the
// matching helper to both rather than open-coding Acquire/release.
func RunGH(ctx context.Context, cmd *exec.Cmd) error {
	release, err := ghThrottle.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	return cmd.Run()
}

func RunGHOutput(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	release, err := ghThrottle.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return cmd.Output()
}

func RunGHCombinedOutput(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	release, err := ghThrottle.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return cmd.CombinedOutput()
}

// RunGHAfterAcquire / RunGHCombinedAfterAcquire mirror the git
// helper runGitCombinedAfterAcquire in apps/backend/internal/worktree:
// the exec timer starts only AFTER the throttle slot is acquired so
// throttle queue time can't burn the per-command budget. Without this
// ordering, a queued waiter inherits its parent's WS-bound deadline,
// gets a slot just before the deadline fires, and is killed with
// `signal: killed` the moment it execs — producing the killed (192×) /
// context deadline exceeded (96×) cascade in the SyncWatchesBatched
// storm logs.
//
// Returns (out, runErr, execCtxErr). The exec-ctx error lets callers
// tell a context-driven kill (timeout) from a regular gh failure when
// classifying fallbacks — see worktree.classifyGitFallbackReason for the
// pattern. The caller is expected to build `cmd` lazily (typically
// `exec.CommandContext(execCtx, "gh", args...)`) via the supplied
// builder closure so the exec context attaches to the right command.
// `execTimeout <= 0` falls back to defaultGHExecTimeout.
func RunGHCombinedAfterAcquire(
	ctx context.Context, execTimeout time.Duration, build func(execCtx context.Context) *exec.Cmd,
) ([]byte, error, error) {
	release, err := ghThrottle.Acquire(ctx)
	if err != nil {
		return nil, err, ctx.Err()
	}
	defer release()
	execCtx, cancel := withExecTimeout(ctx, execTimeout)
	defer cancel()
	cmd := build(execCtx)
	out, runErr := cmd.CombinedOutput()
	return out, runErr, execCtx.Err()
}

// RunGHAfterAcquire is RunGHCombinedAfterAcquire's plain `cmd.Run` sibling.
// Caller owns Stdout/Stderr wiring on the returned `cmd` (build closure
// runs synchronously inside the throttle slot so wiring happens after
// acquire, never before).
func RunGHAfterAcquire(
	ctx context.Context, execTimeout time.Duration, build func(execCtx context.Context) *exec.Cmd,
) (error, error) {
	release, err := ghThrottle.Acquire(ctx)
	if err != nil {
		return err, ctx.Err()
	}
	defer release()
	execCtx, cancel := withExecTimeout(ctx, execTimeout)
	defer cancel()
	cmd := build(execCtx)
	runErr := cmd.Run()
	return runErr, execCtx.Err()
}

// withExecTimeout returns a child context bounded by execTimeout
// (defaultGHExecTimeout when execTimeout<=0). Centralised so the two
// AfterAcquire helpers can't drift on default-timeout behaviour.
func withExecTimeout(ctx context.Context, execTimeout time.Duration) (context.Context, context.CancelFunc) {
	if execTimeout <= 0 {
		execTimeout = defaultGHExecTimeout
	}
	return context.WithTimeout(ctx, execTimeout)
}
