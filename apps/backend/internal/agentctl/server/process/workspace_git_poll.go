package process

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/subproc"
	"go.uber.org/zap"
)

// pollGitChanges periodically checks for git changes (commits, branch switches, staging)
// This catches manual git operations done via shell that file watching might miss.
//
// The tick interval is driven by PollMode (set via SetPollMode), same as
// monitorLoop — see workspace_monitor.go for the wake-up channel design.
func (wt *WorkspaceTracker) pollGitChanges(ctx context.Context) {
	defer wt.wg.Done()

	// If no valid git index was found at startup, git commands will fail.
	// Exit immediately to avoid repeated rev-parse probes on a non-repo directory.
	if wt.gitIndexPath == "" {
		wt.logger.Warn("no valid git repository found, git polling not started",
			zap.String("workDir", wt.workDir))
		return
	}

	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	// Prime cached HEAD SHA, branch name, and index hash. A read failure here
	// is non-fatal: the caches stay empty and the first successful tick fills
	// them, which is what the previous per-value getters did on error too.
	snap, _ := wt.readGitPollSnapshot(ctx)
	wt.gitStateMu.Lock()
	wt.cachedHeadSHA = snap.headSHA
	wt.cachedBranchName = snap.branch
	wt.cachedIndexHash = snap.indexHash
	wt.gitStateMu.Unlock()

	wt.logger.Info("git polling started",
		zap.Duration("fast_interval", wt.gitPollInterval),
		zap.String("initial_mode", string(wt.GetPollMode())),
		zap.String("initial_head", snap.headSHA),
		zap.String("initial_branch", snap.branch))

	var consecutiveFailures int

	for {
		_, gitPoll, _ := wt.pollIntervals(wt.GetPollMode())
		timer.Reset(gitPoll)

		select {
		case <-ctx.Done():
			return
		case <-wt.stopCh:
			return
		case <-wt.gitPollModeChanged:
			if stop := wt.handleGitPollModeChange(ctx, timer, &consecutiveFailures); stop {
				return
			}
		case <-timer.C:
			if stop := wt.handleGitPollTimerTick(ctx, &consecutiveFailures); stop {
				return
			}
		}
	}
}

// handleGitPollModeChange responds to a SetPollMode wake-up: drains the timer
// and runs an immediate scan if we just transitioned into fast mode. Returns
// true if pollGitChanges should exit.
func (wt *WorkspaceTracker) handleGitPollModeChange(ctx context.Context, timer *time.Timer, consecutiveFailures *int) bool {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	if wt.GetPollMode() != PollModeFast {
		return false
	}
	if !atomic.CompareAndSwapInt32(&wt.gitPollRunning, 0, 1) {
		return false
	}
	return wt.gitPollTick(ctx, consecutiveFailures)
}

// handleGitPollTimerTick is the body of a regular timer-driven tick. In paused
// mode it's a no-op (we still wake periodically as a safety net). Returns true
// if pollGitChanges should exit.
func (wt *WorkspaceTracker) handleGitPollTimerTick(ctx context.Context, consecutiveFailures *int) bool {
	if _, _, paused := wt.pollIntervals(wt.GetPollMode()); paused {
		return false
	}
	if !atomic.CompareAndSwapInt32(&wt.gitPollRunning, 0, 1) {
		return false
	}
	return wt.gitPollTick(ctx, consecutiveFailures)
}

// gitPollTick runs a single git poll cycle: checks for manual git operations
// (commits, branch switches, staging). Returns true if the loop should stop.
// The deferred flag reset ensures gitPollRunning is cleared even on panic.
func (wt *WorkspaceTracker) gitPollTick(ctx context.Context, consecutiveFailures *int) bool {
	defer atomic.StoreInt32(&wt.gitPollRunning, 0)

	if !wt.workDirExists() {
		wt.logger.Warn("work directory no longer exists, stopping git polling",
			zap.String("workDir", wt.workDir))
		return true
	}

	snap, err := wt.readGitPollSnapshot(ctx)
	if err != nil {
		return wt.handleGitPollFailure(ctx, consecutiveFailures, err)
	}
	*consecutiveFailures = 0

	wt.checkGitChanges(ctx, snap)
	return false
}

// handleGitPollFailure decides whether a failed snapshot read means git itself
// is broken (e.g. a worktree .git reference pointing at a deleted gitdir) or
// just a transient error worth retrying. The `rev-parse --git-dir` health probe
// used to run on every tick; it now runs only here, because on the happy path
// its exit code is already implied by the snapshot read succeeding.
//
// Returns true if pollGitChanges should exit.
func (wt *WorkspaceTracker) handleGitPollFailure(ctx context.Context, consecutiveFailures *int, cause error) bool {
	probeCmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	probeCmd.Dir = wt.workDir
	if probeErr := subproc.RunGit(ctx, probeCmd); probeErr == nil {
		// The repository is fine, so the snapshot read failed for some other
		// reason (per-command timeout under load, a concurrent index.lock
		// holder). Same as before: only probe failures count against the
		// give-up budget.
		*consecutiveFailures = 0
		wt.logger.Debug("git poll snapshot failed but repository is healthy",
			zap.String("workDir", wt.workDir),
			zap.Error(cause))
		return false
	}

	// If git is broken, stop after maxConsecutiveGitFailures to avoid wasting CPU.
	*consecutiveFailures++
	if *consecutiveFailures >= maxConsecutiveGitFailures {
		wt.logger.Error("git not functional, stopping git polling",
			zap.String("workDir", wt.workDir),
			zap.Int("consecutiveFailures", *consecutiveFailures),
			zap.Error(cause))
		return true
	}
	return false
}

// gitPollSnapshot is one tick's worth of git state: HEAD, branch, and a hash
// standing in for the index contents.
type gitPollSnapshot struct {
	headSHA   string
	branch    string
	indexHash string
}

// Porcelain v2 header prefixes, and the sentinel values git prints when there
// is no commit yet (branch.oid) or HEAD is not on a branch (branch.head). Both
// sentinels map to the empty string, matching what the rev-parse and
// symbolic-ref probes this read replaces returned in those same states.
const (
	gitStatusHeaderPrefix     = "# "
	gitStatusBranchOIDPrefix  = "# branch.oid "
	gitStatusBranchHeadPrefix = "# branch.head "
	gitStatusInitialOID       = "(initial)"
	gitStatusDetachedHead     = "(detached)"
)

// readGitPollSnapshot reads HEAD, branch name and index state in a single git
// invocation. `--porcelain=v2 --branch` emits the `branch.oid` and `branch.head`
// headers alongside the entry lines, which is everything the poll loop
// previously gathered with three separate rev-parse / symbolic-ref / status
// spawns.
//
// Collapsing them matters far more than it looks: on Windows a git spawn pays
// for CreateProcess, the Git-for-Windows shim exec'ing the real binary, and AV
// inspection, all regardless of how little work the command does. Per-tick cost
// therefore tracks the number of spawns, not the work. Measured on a Windows
// dev machine (n=25, medians): 582ms for the four-spawn tick against 176ms for
// this one, with a 146ms floor for a bare `git --version` — i.e. the old tick
// was almost entirely spawn overhead. On macOS and Linux a spawn is 3-5ms and
// none of this is visible, which is why the cost went unnoticed for so long.
//
// Two flags keep --branch from paying for work the poller throws away:
//
//   - --no-ahead-behind, because --branch otherwise walks the revision graph to
//     count how far the branch is from its upstream, and parseGitPollSnapshot
//     discards branch.ab. That walk is not free on a diverged branch: measured
//     at 1799 commits behind, the tick took 334ms with the flag against 646ms
//     without — worse than the four-spawn version this replaces. git prints
//     `# branch.ab +? -?` when it skips the count, which the parser ignores as
//     it ignores every other header.
//   - --untracked-files=no, preserved from the old status call: untracked files
//     are already monitored by monitorLoop via git ls-files, and =all would pay
//     for a full directory traversal on every tick.
func (wt *WorkspaceTracker) readGitPollSnapshot(ctx context.Context) (gitPollSnapshot, error) {
	out, err := wt.runPollingGitOutput(ctx, "status", "--porcelain=v2", "--branch", "--no-ahead-behind", "--untracked-files=no")
	if err != nil {
		return gitPollSnapshot{}, err
	}
	return parseGitPollSnapshot(out), nil
}

// parseGitPollSnapshot splits porcelain v2 output into the branch headers and
// the entry lines. Only the entries feed the hash — headers carry branch.oid
// and branch.ab, which move on every commit and every fetch and would make the
// index hash report changes that the HEAD and branch comparisons already cover.
func parseGitPollSnapshot(out []byte) gitPollSnapshot {
	var snap gitPollSnapshot
	entries := make([]byte, 0, len(out))

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSuffix(line, "\r")
		switch {
		case strings.HasPrefix(line, gitStatusBranchOIDPrefix):
			if oid := strings.TrimPrefix(line, gitStatusBranchOIDPrefix); oid != gitStatusInitialOID {
				snap.headSHA = oid
			}
		case strings.HasPrefix(line, gitStatusBranchHeadPrefix):
			if head := strings.TrimPrefix(line, gitStatusBranchHeadPrefix); head != gitStatusDetachedHead {
				snap.branch = head
			}
		case line == "" || strings.HasPrefix(line, gitStatusHeaderPrefix):
			// Other headers (branch.upstream, branch.ab) are not poll state.
		default:
			entries = append(entries, line...)
			entries = append(entries, '\n')
		}
	}

	hash := sha256.Sum256(entries)
	snap.indexHash = hex.EncodeToString(hash[:])
	return snap
}

// gitPollDelta is a snapshot compared against the cached state: what git says
// now, what it said last tick, and which of the three tracked values moved.
type gitPollDelta struct {
	snap           gitPollSnapshot
	previousHead   string
	previousBranch string
	headChanged    bool
	branchChanged  bool
	indexChanged   bool
}

// compareToCachedState reads the cached HEAD, branch and index hash once and
// diffs the snapshot against them.
func (wt *WorkspaceTracker) compareToCachedState(snap gitPollSnapshot) gitPollDelta {
	wt.gitStateMu.RLock()
	previousHead := wt.cachedHeadSHA
	previousBranch := wt.cachedBranchName
	previousIndexHash := wt.cachedIndexHash
	wt.gitStateMu.RUnlock()

	return gitPollDelta{
		snap:           snap,
		previousHead:   previousHead,
		previousBranch: previousBranch,
		headChanged:    snap.headSHA != "" && snap.headSHA != previousHead,
		// Track all transitions including to/from detached HEAD.
		branchChanged: snap.branch != previousBranch,
		indexChanged:  snap.indexHash != "" && snap.indexHash != previousIndexHash,
	}
}

// checkGitChanges checks if HEAD or git index has changed and processes changes.
// It only routes: each case below owns its own cache update and notifications.
func (wt *WorkspaceTracker) checkGitChanges(ctx context.Context, snap gitPollSnapshot) {
	delta := wt.compareToCachedState(snap)

	switch {
	case !delta.headChanged && !delta.branchChanged && !delta.indexChanged:
		// Nothing moved.
	case delta.indexChanged && !delta.headChanged && !delta.branchChanged:
		wt.handleIndexOnlyChange(ctx, delta)
	case delta.branchChanged && !delta.headChanged:
		wt.handleBranchChangeWithoutHead(ctx, delta)
	case delta.headChanged:
		wt.handleHeadChange(ctx, delta)
	}
}

// handleIndexOnlyChange records staging/unstaging that left HEAD and the branch
// alone.
func (wt *WorkspaceTracker) handleIndexOnlyChange(ctx context.Context, delta gitPollDelta) {
	wt.gitStateMu.Lock()
	wt.cachedIndexHash = delta.snap.indexHash
	wt.gitStateMu.Unlock()

	wt.logger.Debug("git index changed (staging/unstaging detected)")
	wt.tryUpdateGitStatus(ctx)
}

// handleBranchChangeWithoutHead handles a branch move that left HEAD where it
// was. Two ways that happens: switching between branches pointing at the same
// commit, and entering or leaving detached HEAD (git switch --detach, end of a
// rebase).
func (wt *WorkspaceTracker) handleBranchChangeWithoutHead(ctx context.Context, delta gitPollDelta) {
	wt.gitStateMu.Lock()
	wt.cachedBranchName = delta.snap.branch
	wt.cachedIndexHash = delta.snap.indexHash
	wt.gitStateMu.Unlock()

	// In detached HEAD state there is no branch to report a switch to, so just
	// refresh status.
	if delta.snap.branch == "" {
		wt.tryUpdateGitStatus(ctx)
		return
	}

	wt.handleBranchSwitch(ctx, delta.previousBranch, delta.snap.branch, delta.snap.headSHA)
}

// handleHeadChange handles a HEAD move, which is either a branch switch or a
// change in history that needs classifying.
func (wt *WorkspaceTracker) handleHeadChange(ctx context.Context, delta gitPollDelta) {
	wt.logger.Info("git HEAD changed, syncing",
		zap.String("previous", delta.previousHead),
		zap.String("current", delta.snap.headSHA))

	wt.gitStateMu.Lock()
	wt.cachedHeadSHA = delta.snap.headSHA
	wt.cachedBranchName = delta.snap.branch
	wt.cachedIndexHash = delta.snap.indexHash
	wt.gitStateMu.Unlock()

	// Branch switches need special handling to update the base commit. Skipped
	// in detached HEAD state, where there is no branch name.
	if delta.branchChanged && delta.snap.branch != "" {
		wt.handleBranchSwitch(ctx, delta.previousBranch, delta.snap.branch, delta.snap.headSHA)
		return
	}

	if delta.previousHead != "" {
		wt.classifyHeadMovement(ctx, delta)
	}

	// Update and broadcast git status
	wt.tryUpdateGitStatus(ctx)
}

// classifyHeadMovement works out what a HEAD move meant and emits the matching
// notification. There are three cases:
//  1. HEAD moved backward: the new HEAD is an ancestor of the old one (git reset HEAD~1)
//  2. History rewritten: the old HEAD is NOT reachable from the new one (git rebase -i, git commit --amend)
//  3. HEAD moved forward: the old HEAD IS an ancestor of the new one (normal commits)
func (wt *WorkspaceTracker) classifyHeadMovement(ctx context.Context, delta gitPollDelta) {
	switch {
	case wt.isAncestor(ctx, delta.snap.headSHA, delta.previousHead):
		wt.logger.Info("detected git reset (HEAD moved backward)",
			zap.String("previous", delta.previousHead),
			zap.String("current", delta.snap.headSHA))
		wt.notifyWorkspaceStreamGitReset(&types.GitResetNotification{
			Timestamp:      time.Now(),
			RepositoryName: wt.repositoryName,
			PreviousHead:   delta.previousHead,
			CurrentHead:    delta.snap.headSHA,
		})
	case !wt.isAncestor(ctx, delta.previousHead, delta.snap.headSHA):
		wt.logger.Info("detected git history rewrite (previous HEAD not reachable)",
			zap.String("previous", delta.previousHead),
			zap.String("current", delta.snap.headSHA))
		wt.notifyWorkspaceStreamGitReset(&types.GitResetNotification{
			Timestamp:      time.Now(),
			RepositoryName: wt.repositoryName,
			PreviousHead:   delta.previousHead,
			CurrentHead:    delta.snap.headSHA,
		})
	default:
		wt.reportNewCommits(ctx, delta.previousHead)
	}
}

// reportNewCommits notifies about commits made since previousHead, minus the
// ones already on a remote branch — pulling or rebasing onto a remote branch
// would otherwise record upstream commits as session commits.
func (wt *WorkspaceTracker) reportNewCommits(ctx context.Context, previousHead string) {
	commits := wt.getCommitsSince(ctx, previousHead)
	localCommits := wt.filterLocalCommits(ctx, commits)

	for _, commit := range localCommits {
		wt.notifyWorkspaceStreamGitCommit(commit)
	}
	if len(localCommits) > 0 {
		wt.logger.Info("detected new commits via polling",
			zap.Int("count", len(localCommits)))
	}
}

// handleBranchSwitch handles a branch switch event by calculating the new base commit
// and notifying subscribers. This allows the session to update its base commit to reflect
// the new branch's merge-base with the target branch (e.g., main).
func (wt *WorkspaceTracker) handleBranchSwitch(ctx context.Context, previousBranch, currentBranch, currentHead string) {
	wt.logger.Info("detected branch switch",
		zap.String("previous", previousBranch),
		zap.String("current", currentBranch),
		zap.String("head", currentHead))

	// Calculate the new base commit for the current branch
	// Use the sampled currentHead to avoid race conditions
	baseCommit := wt.getBaseCommitForBranch(ctx, currentBranch, currentHead)

	// Only notify if we successfully resolved a base commit
	// Skip notification if base commit resolution failed to avoid incomplete updates
	if baseCommit != "" {
		wt.notifyWorkspaceStreamBranchSwitch(&types.GitBranchSwitchNotification{
			Timestamp:      time.Now(),
			RepositoryName: wt.repositoryName,
			PreviousBranch: previousBranch,
			CurrentBranch:  currentBranch,
			CurrentHead:    currentHead,
			BaseCommit:     baseCommit,
		})
	} else {
		wt.logger.Warn("failed to resolve base commit for branch switch, skipping notification",
			zap.String("branch", currentBranch))
	}

	// Update and broadcast git status to reflect the new branch
	wt.tryUpdateGitStatus(ctx)
}

// getBaseCommitForBranch calculates the base commit (merge-base) for a given branch.
// This is the common ancestor between the branch and the integration branch (e.g., main).
// Prioritizes integration branches (origin/main, origin/master, main, master) over upstream
// to ensure the base commit represents the branch-off point from the main development line.
// Uses the provided head SHA to avoid race conditions with concurrent git operations.
func (wt *WorkspaceTracker) getBaseCommitForBranch(ctx context.Context, branch, head string) string {
	var baseBranch string

	// Try integration branch candidates first (origin/main, origin/master, main, master)
	// This ensures we get the branch-off point from the main development line.
	// Routed through subproc.RunGit so each rev-parse counts against the global
	// git semaphore — handleBranchSwitch can invoke this on every detected
	// branch change, so an unthrottled burst here was the exact pattern the
	// throttle is meant to prevent on CrowdStrike-instrumented macOS.
	for _, candidate := range []string{"origin/main", "origin/master", "main", "master"} {
		checkCmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", candidate)
		checkCmd.Dir = wt.workDir
		if err := subproc.RunGit(ctx, checkCmd); err == nil {
			baseBranch = candidate
			break
		}
	}

	// Fall back to upstream tracking branch if no integration branch exists
	if baseBranch == "" {
		upstreamCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", branch+"@{upstream}")
		upstreamCmd.Dir = wt.workDir
		upstreamOut, err := subproc.RunGitOutput(ctx, upstreamCmd)
		if err == nil && len(upstreamOut) > 0 {
			baseBranch = strings.TrimSpace(string(upstreamOut))
		}
	}

	// Calculate merge-base if we found a base branch
	// Use the sampled head SHA instead of live HEAD to avoid race conditions
	if baseBranch != "" && head != "" {
		mergeBaseCmd := exec.CommandContext(ctx, "git", "merge-base", baseBranch, head)
		mergeBaseCmd.Dir = wt.workDir
		if mergeBaseOut, err := subproc.RunGitOutput(ctx, mergeBaseCmd); err == nil {
			return strings.TrimSpace(string(mergeBaseOut))
		}
	}

	return ""
}

// isAncestor checks if commit1 is an ancestor of commit2
func (wt *WorkspaceTracker) isAncestor(ctx context.Context, commit1, commit2 string) bool {
	cmd := exec.CommandContext(ctx, "git", "merge-base", "--is-ancestor", commit1, commit2)
	cmd.Dir = wt.workDir
	err := subproc.RunGit(ctx, cmd)
	// Exit code 0 means commit1 IS an ancestor of commit2
	// Exit code 1 means commit1 is NOT an ancestor of commit2
	return err == nil
}

// isOnRemote checks if a commit is reachable from any remote tracking branch.
// This is used to filter out upstream commits that came from a pull/fetch,
// as opposed to commits made locally in the session.
func (wt *WorkspaceTracker) isOnRemote(ctx context.Context, commitSHA string) bool {
	// Use git branch -r --contains to check if commit is on any remote branch
	cmd := exec.CommandContext(ctx, "git", "branch", "-r", "--contains", commitSHA)
	cmd.Dir = wt.workDir
	out, err := subproc.RunGitOutput(ctx, cmd)
	if err != nil {
		// If the command fails, assume it's not on remote (safer default)
		return false
	}
	// If output is non-empty, commit is reachable from at least one remote branch
	return strings.TrimSpace(string(out)) != ""
}

// filterLocalCommits filters out commits that are already on remote branches.
// This ensures we only report commits made locally in the session, not upstream commits
// that came from pulling/rebasing onto a remote branch.
func (wt *WorkspaceTracker) filterLocalCommits(ctx context.Context, commits []*types.GitCommitNotification) []*types.GitCommitNotification {
	if len(commits) == 0 {
		return commits
	}

	localCommits := make([]*types.GitCommitNotification, 0, len(commits))
	for _, commit := range commits {
		if !wt.isOnRemote(ctx, commit.CommitSHA) {
			localCommits = append(localCommits, commit)
		} else {
			wt.logger.Debug("skipping upstream commit (already on remote)",
				zap.String("sha", commit.CommitSHA),
				zap.String("message", commit.Message))
		}
	}

	if skipped := len(commits) - len(localCommits); skipped > 0 {
		wt.logger.Info("filtered upstream commits",
			zap.Int("total", len(commits)),
			zap.Int("skipped", skipped),
			zap.Int("local", len(localCommits)))
	}

	return localCommits
}
