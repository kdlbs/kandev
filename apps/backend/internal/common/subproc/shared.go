package subproc

import (
	"context"
	"os"
	"os/exec"
	"strconv"
)

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
	ghThrottle  = NewThrottle(resolveCap(ghMaxConcurrentEnv, defaultGHMaxConcurrent))
	gitThrottle = NewThrottle(resolveCap(gitMaxConcurrentEnv, defaultGitMaxConcurrent))
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

// RunGit acquires a git slot, runs cmd, and releases the slot. Use
// this anywhere we exec a git binary — calling cmd.Run() directly
// bypasses the throttle. The caller owns cmd.Stdout/Stderr wiring.
func RunGit(ctx context.Context, cmd *exec.Cmd) error {
	release, err := gitThrottle.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	return cmd.Run()
}

// RunGitCombinedOutput is RunGit's CombinedOutput sibling.
func RunGitCombinedOutput(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	release, err := gitThrottle.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return cmd.CombinedOutput()
}

// RunGitOutput is RunGit's Output sibling. stderr ends up in
// (*exec.ExitError).Stderr when set by the caller.
func RunGitOutput(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	release, err := gitThrottle.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return cmd.Output()
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
