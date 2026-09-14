package process

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/fsdiagnostics"
	"github.com/kandev/kandev/internal/common/subproc"
	"go.uber.org/zap"
)

// safeBranchRefPattern is the inline allowlist used by the git-status
// sinks below. Declared at package scope so each sink site reads as a
// direct `regexp.MatchString` call — CodeQL's `go/command-injection`
// taint tracker treats that pattern as a sanitiser barrier in a way it
// did not recognise when the same check sat behind a helper function.
// Matches the regex `securityutil.IsValidBranchName` uses; behaviour is
// unchanged.
var safeBranchRefPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`)

// updateGitStatus updates the git status. Callers must coordinate access
// via updateMu — use tryUpdateGitStatus for polling loops, RefreshGitStatus
// for user-triggered operations. Returns whether a new status was computed
// and published; false on a getGitStatus failure or a context already
// cancelled, so callers that must not lose the event they're reacting to
// (see tryUpdateGitStatus) know a retry is needed.
func (wt *WorkspaceTracker) updateGitStatus(ctx context.Context) bool {
	return wt.updateGitStatusClass(ctx, gitWorkClass(ctx))
}

func (wt *WorkspaceTracker) updateGitStatusClass(ctx context.Context, class subproc.GitWorkClass) bool {
	status, err := wt.getGitStatusClass(ctx, class)
	if err != nil {
		// A cancellation is the expected outcome when the tracker is being
		// torn down (Stop cancels cancelCtx, which kills the in-flight git
		// command) or when the poll's admission slot is revoked. That is not a
		// failure worth a warning — it mirrors handleGitPollFailure, which also
		// downgrades cancellation-class errors to Debug to keep shutdown quiet.
		if wt.isGitStatusCancellation(err) {
			wt.logger.Debug("updateGitStatus: getGitStatus canceled", zap.Error(err))
		} else {
			if fsdiagnostics.IsAccessDenied(err) {
				wt.recordFilesystemFailure("workspace.git_status", workspaceTrigger(ctx, "poll"), err)
			} else {
				wt.logger.Warn("updateGitStatus: getGitStatus failed", zap.Error(err))
			}
		}
		return false
	}
	if ctx.Err() != nil || (wt.cancelCtx != nil && wt.cancelCtx.Err() != nil) {
		return false
	}

	wt.mu.Lock()
	wt.currentStatus = status
	wt.mu.Unlock()

	// Notify workspace stream subscribers
	wt.notifyWorkspaceStreamGitStatus(status)
	return true
}

// isGitStatusCancellation reports whether a getGitStatus error is a benign
// cancellation rather than a real git failure: the passed context or the
// tracker's own shutdown context was canceled, or the background admission slot
// was revoked. These are all expected during teardown and don't warrant a warn.
func (wt *WorkspaceTracker) isGitStatusCancellation(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, subproc.ErrAdmissionCanceled) {
		return true
	}
	return wt.cancelCtx != nil && wt.cancelCtx.Err() != nil
}

// tryUpdateGitStatus attempts a non-blocking git status update. If another
// update is already in progress (from the other polling loop or an explicit
// refresh), the call is skipped — the running update will produce the same
// result. Returns whether it actually published a new status: false either
// because it was skipped (lock contention) or because updateGitStatus itself
// failed. Callers reacting to a specific change (e.g. an upstream ref move)
// must check this before treating that change as observed — see
// handleUpstreamOnlyChange in workspace_git_poll.go.
func (wt *WorkspaceTracker) tryUpdateGitStatus(ctx context.Context) bool {
	if !wt.updateMu.TryLock() {
		return false
	}
	defer wt.updateMu.Unlock()
	return wt.updateGitStatusClass(ctx, subproc.GitBackground)
}

// RefreshGitStatus forces a git status refresh and notifies subscribers.
// Useful after index-only changes (stage/unstage) that the file watcher won't detect.
// Uses a blocking lock so user-triggered operations always complete.
func (wt *WorkspaceTracker) RefreshGitStatus(ctx context.Context) {
	wt.updateMu.Lock()
	defer wt.updateMu.Unlock()
	wt.updateGitStatusClass(ctx, subproc.GitInteractive)
}

// RefreshWorkspace performs one file and Git scan for a lifecycle boundary or
// an explicit user retry. The update lock keeps the two snapshots coherent
// with the normal polling loops.
func (wt *WorkspaceTracker) RefreshWorkspace(ctx context.Context, trigger string) {
	if err := ctx.Err(); err != nil {
		return
	}
	wt.clearAccessDeniedForUserOperation(trigger)
	if trigger == workspaceManualRefreshTrigger || trigger == workspaceUserSelectTrigger {
		wt.SetPollMode(PollModeFast)
	}
	ctx = withWorkspaceTrigger(ctx, trigger)
	wt.updateMu.Lock()
	defer wt.updateMu.Unlock()
	wt.updateGitStatusClass(ctx, subproc.GitInteractive)
	if err := ctx.Err(); err != nil {
		return
	}
	wt.updateFilesClass(ctx, subproc.GitInteractive)
	if err := ctx.Err(); err != nil {
		return
	}
	wt.notifyWorkspaceStreamFileChange(types.FileChangeNotification{
		Timestamp:      time.Now(),
		RepositoryName: wt.repositoryName,
		Operation:      types.FileOpRefresh,
	})
}

// GetCurrentGitStatus returns the current cached git status. If no status has
// been cached yet, it starts or joins a live observation without caching it.
func (wt *WorkspaceTracker) GetCurrentGitStatus(ctx context.Context) (types.GitStatusUpdate, error) {
	return wt.GetGitStatus(ctx, false)
}

// GetGitStatus returns git status. When fresh is false, the cached value is
// returned (with a live fallback if no cache exists). When fresh is true, the
// caller starts or joins a live observation while bypassing the cache. Callers
// that need up-to-date data after the agent just committed changes should pass
// fresh=true.
func (wt *WorkspaceTracker) GetGitStatus(ctx context.Context, fresh bool) (types.GitStatusUpdate, error) {
	if fresh {
		return wt.getGitStatusClass(ctx, subproc.GitInteractive)
	}

	wt.mu.RLock()
	status := wt.currentStatus
	wt.mu.RUnlock()

	if status.Timestamp.IsZero() {
		return wt.getGitStatusClass(ctx, subproc.GitInteractive)
	}

	return status, nil
}

// getGitStatus coalesces live status observations for this tracker. The
// computation is owned by the tracker rather than the first caller, while each
// waiter can still return promptly when its own context is canceled.
func (wt *WorkspaceTracker) getGitStatus(ctx context.Context) (types.GitStatusUpdate, error) {
	return wt.getGitStatusClass(ctx, gitWorkClass(ctx))
}

func (wt *WorkspaceTracker) getGitStatusClass(ctx context.Context, class subproc.GitWorkClass) (types.GitStatusUpdate, error) {
	if err := ctx.Err(); err != nil {
		return types.GitStatusUpdate{}, err
	}

	// Observations are coalesced only within the same admission class. A
	// background poll already in flight must not capture a fresh interactive
	// request and make its Git commands run on the background queue.
	key := "live:" + string(class)
	resultCh := wt.gitStatusGroup.DoChan(key, func() (interface{}, error) {
		sharedCtx, finish, err := wt.beginGitStatusObservation()
		if err != nil {
			return types.GitStatusUpdate{}, err
		}
		defer finish()

		observer := wt.gitStatusObserver
		if observer == nil {
			observer = wt.computeGitStatus
		}
		status, err := observer(withGitWorkClass(sharedCtx, class))
		if err != nil {
			return types.GitStatusUpdate{}, err
		}
		if err := sharedCtx.Err(); err != nil {
			return types.GitStatusUpdate{}, err
		}
		return status, nil
	})
	if wt.gitStatusWaiterJoined != nil {
		wt.gitStatusWaiterJoined()
	}

	select {
	case <-ctx.Done():
		return types.GitStatusUpdate{}, ctx.Err()
	case result := <-resultCh:
		if err := ctx.Err(); err != nil {
			return types.GitStatusUpdate{}, err
		}
		if result.Err != nil {
			return types.GitStatusUpdate{}, result.Err
		}
		status, ok := result.Val.(types.GitStatusUpdate)
		if !ok {
			return types.GitStatusUpdate{}, fmt.Errorf("unexpected git status observation result %T", result.Val)
		}
		return status, nil
	}
}

// beginGitStatusObservation admits one singleflight body into the tracker
// lifecycle. Stop holds the same mutex while canceling tracker context, which
// prevents WaitGroup Add from racing with shutdown's Wait.
func (wt *WorkspaceTracker) beginGitStatusObservation() (context.Context, func(), error) {
	wt.gitStatusObserveMu.Lock()
	defer wt.gitStatusObserveMu.Unlock()

	baseCtx := wt.cancelCtx
	if baseCtx != nil {
		if err := baseCtx.Err(); err != nil {
			return nil, nil, err
		}
	} else {
		// Preserve the pre-singleflight zero-value behavior for direct status
		// reads. Constructed trackers always use their cancellable lifetime.
		baseCtx = context.Background()
	}

	timeout := wt.gitStatusObserveTimeout
	if timeout == 0 && wt.cancelCtx == nil {
		timeout = workspaceGitStatusObserveTimeout
	}
	sharedCtx, cancel := context.WithTimeout(baseCtx, timeout)
	wt.gitStatusObserveWG.Add(1)
	return sharedCtx, func() {
		cancel()
		wt.gitStatusObserveWG.Done()
	}, nil
}

// computeGitStatus retrieves the current git status. Secondary commands
// (ahead/behind counts, diff enrichment) that fail or time out carry their
// values forward from the prior cached status rather than overwriting them
// with zero. The two primary commands (branch info and `git status
// --porcelain`) still propagate errors upward; on those, updateGitStatus
// skips the cache write entirely so the prior state stays visible to the UI.
func (wt *WorkspaceTracker) computeGitStatus(ctx context.Context) (types.GitStatusUpdate, error) {
	update := types.GitStatusUpdate{
		Timestamp:      time.Now(),
		RepositoryName: wt.repositoryName,
		IsSubmodule:    wt.IsSubmodule(),
		Modified:       []string{},
		Added:          []string{},
		Deleted:        []string{},
		Untracked:      []string{},
		Renamed:        []string{},
		Files:          make(map[string]types.FileInfo),
	}
	comparison := wt.ComparisonResolution()
	if comparison.Explicit {
		update.ComparisonTarget = comparison.Display
		update.ComparisonStatus = comparison.Status
		update.ComparisonErrorCode = comparison.ErrorCode
	}

	// Bare trackers (multi-repo task roots) sit on a directory that isn't
	// itself a git repo. Without this guard, `git status` would ascend the
	// directory tree until it found a `.git` — for tasks nested inside a
	// developer's own kandev checkout that lands on the OUTER worktree and
	// silently emits its branch/ahead/behind as if it were the task. Bail
	// out early to keep the bare tracker's `currentStatus` zero-valued.
	if wt.gitIndexPath == "" {
		return update, nil
	}

	// Snapshot the prior cache once so per-command carry-forward sees a
	// consistent view even if a concurrent RefreshGitStatus replaces
	// currentStatus mid-poll.
	wt.mu.RLock()
	prior := wt.currentStatus
	wt.mu.RUnlock()

	if err := wt.getGitBranchInfo(ctx, &update); err != nil {
		return update, err
	}
	if comparison.Explicit && comparison.Status == comparisonTargetStatusReady && update.BaseCommit == "" {
		update.ComparisonStatus = comparisonTargetStatusUnavailable
		update.ComparisonErrorCode = comparisonTargetErrorMergeBase
	}
	if err := ctx.Err(); err != nil {
		return update, err
	}

	wt.getAheadBehindCounts(ctx, &update, prior)
	if err := ctx.Err(); err != nil {
		return update, err
	}

	wt.getRemoteAheadBehindCounts(ctx, &update, prior)
	if err := ctx.Err(); err != nil {
		return update, err
	}

	if err := wt.parseGitStatusOutput(ctx, &update); err != nil {
		return update, err
	}
	if err := ctx.Err(); err != nil {
		return update, err
	}

	// Enrich file info with diff data (additions, deletions, and actual diff content)
	if err := wt.enrichWithDiffData(ctx, &update, prior); err != nil {
		return update, err
	}
	if err := ctx.Err(); err != nil {
		return update, err
	}

	// Compute full branch totals vs merge-base (committed + staged + unstaged + untracked)
	if err := wt.enrichWithBranchDiff(ctx, &update, prior); err != nil {
		return types.GitStatusUpdate{}, err
	}
	if err := ctx.Err(); err != nil {
		return update, err
	}

	return update, nil
}

// getGitBranchInfo populates branch, remote branch, head commit, and base commit fields.
// Each command runs under a per-command timeout via the runGit* helpers so a
// single wedged git invocation cannot pin the shared throttle slot.
func (wt *WorkspaceTracker) getGitBranchInfo(ctx context.Context, update *types.GitStatusUpdate) error {
	// Get current branch
	branchOut, err := wt.runGitOutput(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	update.Branch = strings.TrimSpace(string(branchOut))

	// Get remote branch. A non-nil error here is the normal, expected case
	// for a branch with no upstream yet (not logged — this fires on every
	// poll of every unpushed branch).
	if remoteOut, err := wt.runGitOutput(ctx, "rev-parse", "--abbrev-ref", "@{upstream}"); err == nil {
		update.RemoteBranch = strings.TrimSpace(string(remoteOut))
	}

	// Get current HEAD commit SHA
	if headOut, err := wt.runGitOutput(ctx, "rev-parse", "HEAD"); err == nil {
		update.HeadCommit = strings.TrimSpace(string(headOut))
	}

	// Get base commit SHA using merge-base between current branch and the
	// integration branch. The task's recorded base_branch (if any) wins;
	// otherwise fall back to the integration branch (origin/main, etc.)
	// rather than the tracking branch, so we show all changes this branch
	// introduces compared to the main development line. The stored ref may
	// be absent in the local clone (e.g. a feature branch never fetched);
	// `git rev-parse --verify` filters those automatically.
	//
	// When `git merge-base` fails (typically because the branches share no
	// history — local backups, freshly imported repos, etc.) we fall back
	// to the branch tip itself. This keeps the diff/log range anchored
	// against the ref the user actually picked instead of going empty,
	// which the agentctl git-log handler used to silently translate into
	// "last N commits of HEAD" (the symptom in the no-merge-base repro:
	// `+1 -0` on the card vs. 100 unrelated commits in the panel).
	update.BaseCommit = wt.ResolveBaseCommit(ctx)

	return nil
}

// computeBaseCommit resolves the SHA the workspace_git_diff numstat command
// uses as its diff anchor. Prefers merge-base(baseBranch, HEAD) because that
// matches "changes this branch introduces"; falls back to the tip of
// baseBranch when no merge-base exists so we still produce a stable anchor
// for unrelated-history cases. Returns "" only if both lookups fail.
//
// baseBranch is treated as user-controlled and re-sanitised here so static
// analysis sees the regex barrier inline with the `git` invocation.
func (wt *WorkspaceTracker) computeBaseCommit(ctx context.Context, baseBranch string) string {
	if wt.IsSubmodule() {
		if !sha1HexPattern.MatchString(baseBranch) {
			return ""
		}
		out, err := wt.runGitOutput(ctx, "rev-parse", "--verify", baseBranch+"^{commit}")
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	// Same inline regex barrier as resolveStoredRef so CodeQL's
	// taint-tracker sees it co-located with the `git` subprocess call.
	rest, hasOriginPrefix := strings.CutPrefix(baseBranch, "origin/")
	check := baseBranch
	if hasOriginPrefix {
		check = rest
	}
	if !safeBranchRefPattern.MatchString(check) || strings.Contains(check, "..") || strings.HasSuffix(check, ".lock") {
		return ""
	}
	if out, err := wt.runGitOutput(ctx, "merge-base", baseBranch, "HEAD"); err == nil {
		if sha := strings.TrimSpace(string(out)); sha != "" {
			return correctStaleComparisonBase(
				ctx,
				workspaceTrackerComparisonGit{tracker: wt},
				sha,
				baseBranch,
			)
		}
	}
	if out, err := wt.runGitOutput(ctx, "rev-parse", baseBranch); err == nil {
		base := strings.TrimSpace(string(out))
		return correctStaleComparisonBase(
			ctx,
			workspaceTrackerComparisonGit{tracker: wt},
			base,
			baseBranch,
		)
	}
	return ""
}

// ResolveBaseCommit returns the comparison anchor configured for this
// repository. It shares the exact stored-base and fallback resolution used by
// git status so API consumers such as commits and cumulative diff cannot drift
// from the Changes panel's ahead/behind and branch-diff totals.
func (wt *WorkspaceTracker) ResolveBaseCommit(ctx context.Context) string {
	baseBranch := wt.resolveBaseBranch(ctx)
	if baseBranch == "" {
		return ""
	}
	return wt.computeBaseCommit(ctx, baseBranch)
}

// ResolveBaseAnchor returns both the comparison anchor SHA and the base branch
// ref it was computed against. Multi-repo commits/diff use the ref to detect a
// stale local-only base (merged/deleted stacked parent) the same way the
// single-repo path uses target_branch. Both are "" when no base resolves.
func (wt *WorkspaceTracker) ResolveBaseAnchor(ctx context.Context) (sha, baseBranch string) {
	baseBranch = wt.resolveBaseBranch(ctx)
	if baseBranch == "" {
		return "", ""
	}
	return wt.computeBaseCommit(ctx, baseBranch), baseBranch
}

// getAheadBehindCounts populates the Ahead/Behind fields relative to the base
// branch (origin/main or origin/master). Always compares against the base
// branch rather than the remote tracking branch, because after a rebase the
// tracking branch has stale commit SHAs that produce inflated counts.
//
// If either the compareRef lookup or the count command fails (e.g. timeout
// under throttle saturation, transient FS hang), Ahead/Behind are carried
// forward from prior when HEAD hasn't moved. Otherwise a single bad poll
// would silently overwrite the cached counts with 0/0 and hide a legitimate
// "Pull N" / "Push N" indicator in the UI.
func (wt *WorkspaceTracker) getAheadBehindCounts(ctx context.Context, update *types.GitStatusUpdate, prior types.GitStatusUpdate) {
	comparison := wt.ComparisonResolution()
	if comparison.Explicit && comparison.Status != comparisonTargetStatusReady {
		return
	}
	// Always compare against the base branch (task-stored value if set,
	// otherwise origin/main / origin/master). Using the remote tracking
	// branch (origin/<feature-branch>) gives wrong counts after rebase
	// because rebased commits have new SHAs.
	compareRef := wt.resolveAheadBehindRef(ctx)
	// Inline regex allowlist so the sanitiser barrier sits in the same
	// function as the `git rev-list` invocation below.
	rest, hasOriginPrefix := strings.CutPrefix(compareRef, "origin/")
	check := compareRef
	if hasOriginPrefix {
		check = rest
	}
	if !safeBranchRefPattern.MatchString(check) || strings.Contains(check, "..") || strings.HasSuffix(check, ".lock") {
		compareRef = ""
	}
	if compareRef == "" {
		if comparison.Explicit {
			update.ComparisonStatus = comparisonTargetStatusUnavailable
			update.ComparisonErrorCode = comparisonTargetErrorRefUnavailable
			return
		}
		carryAheadBehind(update, prior)
		return
	}
	countOut, err := wt.runGitOutput(ctx, "rev-list", "--left-right", "--count", update.Branch+"..."+compareRef)
	if err != nil {
		if comparison.Explicit {
			update.ComparisonStatus = comparisonTargetStatusUnavailable
			update.ComparisonErrorCode = comparisonTargetErrorRefUnavailable
			return
		}
		wt.logger.Debug("getAheadBehindCounts: rev-list failed, carrying forward", zap.Error(err))
		carryAheadBehind(update, prior)
		return
	}
	parts := strings.Fields(string(countOut))
	if len(parts) != 2 {
		if comparison.Explicit {
			update.ComparisonStatus = comparisonTargetStatusUnavailable
			update.ComparisonErrorCode = comparisonTargetErrorRefUnavailable
			return
		}
		carryAheadBehind(update, prior)
		return
	}
	ahead, aheadErr := strconv.Atoi(parts[0])
	behind, behindErr := strconv.Atoi(parts[1])
	if aheadErr != nil || behindErr != nil || ahead < 0 || behind < 0 {
		if comparison.Explicit {
			update.ComparisonStatus = comparisonTargetStatusUnavailable
			update.ComparisonErrorCode = comparisonTargetErrorRefUnavailable
			return
		}
		carryAheadBehind(update, prior)
		return
	}
	update.Ahead = ahead
	update.Behind = behind
}

// getRemoteAheadBehindCounts populates RemoteAhead/RemoteBehind relative to
// this branch's own upstream (@{upstream}) — the counts push-detection needs
// to tell "has this branch been pushed" apart from Ahead/Behind, which are
// deliberately base-branch-relative (see getAheadBehindCounts) and never
// reach zero just because a push happened. Left at 0/0 when RemoteBranch is
// empty (no upstream configured yet — nothing to compare against).
//
// Same carry-forward-on-failure treatment as getAheadBehindCounts: a
// transient rev-list failure keeps the prior counts instead of flashing 0/0
// (which would look like "just pushed" to a push-detection consumer).
func (wt *WorkspaceTracker) getRemoteAheadBehindCounts(ctx context.Context, update *types.GitStatusUpdate, prior types.GitStatusUpdate) {
	if update.RemoteBranch == "" {
		update.RemoteHeadCommit = ""
		update.RemoteAhead = 0
		update.RemoteBehind = 0
		return
	}
	// RemoteBranch came from Git, but keep the same inline ref boundary as the
	// other status comparisons before using it in a command argument.
	rest, hasOriginPrefix := strings.CutPrefix(update.RemoteBranch, "origin/")
	check := update.RemoteBranch
	if hasOriginPrefix {
		check = rest
	}
	if !safeBranchRefPattern.MatchString(check) || strings.Contains(check, "..") || strings.HasSuffix(check, ".lock") {
		carryRemoteSnapshot(update, prior)
		return
	}
	remoteHeadOut, err := wt.runGitOutput(ctx, "rev-parse", "--verify", update.RemoteBranch+"^{commit}")
	if err != nil {
		wt.logger.Debug("getRemoteAheadBehindCounts: rev-parse failed, carrying forward", zap.Error(err))
		carryRemoteSnapshot(update, prior)
		return
	}
	remoteHead := strings.TrimSpace(string(remoteHeadOut))
	countOut, err := wt.runGitOutput(ctx, "rev-list", "--left-right", "--count", "HEAD..."+remoteHead)
	if err != nil {
		wt.logger.Debug("getRemoteAheadBehindCounts: rev-list failed, carrying forward", zap.Error(err))
		carryRemoteSnapshot(update, prior)
		return
	}
	parts := strings.Fields(string(countOut))
	if len(parts) != 2 {
		carryRemoteSnapshot(update, prior)
		return
	}
	update.RemoteHeadCommit = remoteHead
	update.RemoteAhead, _ = strconv.Atoi(parts[0])
	update.RemoteBehind, _ = strconv.Atoi(parts[1])
}

// branchDiffCandidates is the integration-branch priority list used when the
// task has no recorded base_branch (legacy tasks / external branches). Kept
// in sync between base-commit and ahead/behind resolution.
var branchDiffCandidates = integrationBranchRefs(true)

// aheadBehindFallbackCandidates is the subset of branchDiffCandidates used by
// ahead/behind counts. Local main/master are intentionally excluded — the
// counts are meant to reflect divergence from the remote integration line,
// and falling back to a local branch can show stale, in-progress work.
var aheadBehindFallbackCandidates = integrationBranchRefs(false)

// resolveBaseBranch picks the ref that workspace_git_diff uses as the
// merge-base for BranchAdditions/BranchDeletions. Prefers the task's
// recorded base_branch (set by the process manager from
// task_repositories.base_branch); when that's missing or no longer exists
// in git, falls back to the integration-branch priority list.
//
// For each candidate the upstream remote ref (`origin/<name>`) is tried
// before the bare local ref — matches computeMergeBase in
// agentctl/server/api/git.go so the task-card stats and the commits panel
// land on the same merge-base. Without this alignment the picker case
// shows e.g. `+1 -0` against the local branch tip while the commits panel
// shows the full 100-commit range against a stale upstream — a confusing
// stats/commits mismatch even though both sides are computing correctly
// for the ref name they happened to resolve first.
func (wt *WorkspaceTracker) resolveBaseBranch(ctx context.Context) string {
	resolution := wt.resolveBaseBranchWithReason(ctx)
	resolution.log(wt)
	return resolution.ref
}

// baseBranchReason records why resolveBaseBranch landed on the ref it did.
// The two fallback reasons have different fixes — one means the task's base
// branch never reached this tracker, the other means it did but no longer
// exists in git — so a wrong diff stat must be attributable to one of them
// without re-deriving the resolution by hand.
type baseBranchReason int

const (
	baseBranchStored baseBranchReason = iota
	baseBranchFallbackNoStored
	baseBranchFallbackStoredUnresolved
	baseBranchUnresolved
)

// baseBranchResolution is resolveBaseBranch's decision plus the evidence
// needed to explain it.
type baseBranchResolution struct {
	ref    string
	stored string
	reason baseBranchReason
}

// log reports a fallback resolution once per change. The stored case is the
// norm and stays silent.
//
// Two constraints shape this. resolveBaseBranch runs on every status poll — as
// often as every couple of seconds, per repository — so logging unconditionally
// would bury the signal in its own repetition; only a *changed* outcome is
// reported. And a recorded base branch that does not resolve is an anomaly the
// operator needs to see without turning on debug logging, so it warns, while a
// task that simply has no recorded base is ordinary and stays at debug.
func (r baseBranchResolution) log(wt *WorkspaceTracker) {
	key := strconv.Itoa(int(r.reason)) + "|" + r.stored + "|" + r.ref
	wt.baseBranchLogMu.Lock()
	repeat := wt.lastBaseBranchLog == key
	wt.lastBaseBranchLog = key
	wt.baseBranchLogMu.Unlock()
	if repeat || r.reason == baseBranchStored {
		return
	}
	repository := wt.repositoryName
	if repository == "" && wt.workDir != "" {
		repository = filepath.Base(filepath.Clean(wt.workDir))
	}

	switch r.reason {
	case baseBranchFallbackNoStored:
		wt.logger.Debug("no base branch recorded for workspace, using integration fallback for diff stats",
			zap.String("repository", repository),
			zap.String("candidate", r.ref))
	case baseBranchFallbackStoredUnresolved:
		wt.logger.Warn("recorded base branch does not resolve in git, diff stats fall back to an integration branch",
			zap.String("repository", repository),
			zap.String("stored_base_branch", r.stored),
			zap.String("candidate", r.ref))
	case baseBranchUnresolved:
		wt.logger.Warn("no base branch or integration candidate resolved, diff stats unavailable",
			zap.String("repository", repository),
			zap.String("stored_base_branch", r.stored))
	case baseBranchStored:
	}
}

// resolveBaseBranchWithReason is resolveBaseBranch's decision, separated so the
// outcome is assertable in tests rather than only observable through logs.
func (wt *WorkspaceTracker) resolveBaseBranchWithReason(ctx context.Context) baseBranchResolution {
	comparison := wt.ComparisonResolution()
	if comparison.Explicit {
		if comparison.Status == comparisonTargetStatusReady && comparison.Ref != "" {
			return baseBranchResolution{ref: comparison.Ref, stored: comparison.Display, reason: baseBranchStored}
		}
		return baseBranchResolution{stored: comparison.Display, reason: baseBranchUnresolved}
	}
	stored := wt.BaseBranch()
	if wt.IsSubmodule() {
		anchor := wt.ComparisonAnchor()
		if anchor != "" {
			if ref := wt.resolveStoredRef(ctx, anchor); ref != "" {
				return baseBranchResolution{ref: ref, stored: anchor, reason: baseBranchStored}
			}
		}
		return baseBranchResolution{stored: anchor, reason: baseBranchUnresolved}
	}
	if stored != "" {
		if ref := wt.resolveStoredRef(ctx, stored); ref != "" {
			return baseBranchResolution{ref: ref, stored: stored, reason: baseBranchStored}
		}
	}
	fallbackReason := baseBranchFallbackNoStored
	if stored != "" {
		fallbackReason = baseBranchFallbackStoredUnresolved
	}
	for _, candidate := range branchDiffCandidates {
		if err := wt.runGit(ctx, "rev-parse", "--verify", candidate); err == nil {
			return baseBranchResolution{ref: candidate, stored: stored, reason: fallbackReason}
		}
	}
	return baseBranchResolution{stored: stored, reason: baseBranchUnresolved}
}

// resolveAheadBehindRef is the ahead/behind variant of resolveBaseBranch.
// It uses the stored base_branch first, then the smaller
// aheadBehindFallbackCandidates list — local main/master are excluded
// because they can show stale, in-progress work for divergence counts.
func (wt *WorkspaceTracker) resolveAheadBehindRef(ctx context.Context) string {
	comparison := wt.ComparisonResolution()
	if comparison.Explicit {
		if comparison.Status == comparisonTargetStatusReady {
			return comparison.Ref
		}
		return ""
	}
	if wt.IsSubmodule() {
		return wt.resolveBaseBranch(ctx)
	}
	if stored := wt.BaseBranch(); stored != "" {
		if ref := wt.resolveStoredRef(ctx, stored); ref != "" {
			return ref
		}
	}
	for _, candidate := range aheadBehindFallbackCandidates {
		if err := wt.runGit(ctx, "rev-parse", "--verify", candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// resolveStoredRef expands a task-stored base_branch into the actual ref
// the merge-base / diff commands should use. Tries `origin/<name>` first
// (the upstream source of truth — local refs can lag arbitrarily far behind
// on long-lived worktrees), then the bare name as a fallback for refs that
// only live locally. Refs that already carry the `origin/` prefix are
// passed through unchanged so callers can opt out of the remote-first
// behavior by storing the full `origin/<name>` value.
//
// `stored` originates from user-controlled input (the picker payload or a
// task_repositories row). It is regex-sanitised here BEFORE flowing into
// any `git` arg so CodeQL's `go/command-injection` taint tracker sees a
// fresh sanitiser barrier right at the call site — even though
// SetBaseBranch already rejected unsafe values when the field was stored.
func (wt *WorkspaceTracker) resolveStoredRef(ctx context.Context, stored string) string {
	// Inline regexp.MatchString call at the call site so CodeQL's
	// `go/command-injection` taint tracker sees the allowlist barrier in
	// the same function as the `wt.runGit` invocations below. The pattern
	// matches `securityutil.IsValidBranchName`'s allowlist.
	rest, hasOriginPrefix := strings.CutPrefix(stored, "origin/")
	check := stored
	if hasOriginPrefix {
		check = rest
	}
	if !safeBranchRefPattern.MatchString(check) || strings.Contains(check, "..") || strings.HasSuffix(check, ".lock") {
		return ""
	}
	if hasOriginPrefix {
		if err := wt.runGit(ctx, "rev-parse", "--verify", stored); err == nil {
			return stored
		}
		return ""
	}
	origin := "origin/" + stored
	if err := wt.runGit(ctx, "rev-parse", "--verify", origin); err == nil {
		return origin
	}
	if err := wt.runGit(ctx, "rev-parse", "--verify", stored); err == nil {
		return stored
	}
	return ""
}

// carryAheadBehind copies prior.Ahead/Behind onto update when HEAD hasn't
// moved. A new HEAD invalidates prior counts (the user committed, pulled, or
// reset), so we leave update zeroed in that case.
func carryAheadBehind(update *types.GitStatusUpdate, prior types.GitStatusUpdate) {
	if prior.HeadCommit == "" || prior.HeadCommit != update.HeadCommit {
		return
	}
	update.Ahead = prior.Ahead
	update.Behind = prior.Behind
}

// carryRemoteAheadBehind mirrors carryAheadBehind for RemoteAhead/RemoteBehind.
func carryRemoteAheadBehind(update *types.GitStatusUpdate, prior types.GitStatusUpdate) {
	if prior.HeadCommit == "" || prior.HeadCommit != update.HeadCommit {
		return
	}
	update.RemoteAhead = prior.RemoteAhead
	update.RemoteBehind = prior.RemoteBehind
}

// carryRemoteSnapshot keeps the upstream tip and divergence counts coherent
// when one of the secondary upstream observations fails. A changed local HEAD
// or tracking ref makes the previous snapshot unsafe, so the caller keeps the
// zero-value unknown state instead.
func carryRemoteSnapshot(update *types.GitStatusUpdate, prior types.GitStatusUpdate) {
	update.RemoteHeadCommit = ""
	update.RemoteAhead = 0
	update.RemoteBehind = 0
	if prior.HeadCommit == "" || prior.HeadCommit != update.HeadCommit {
		return
	}
	if prior.RemoteBranch == "" || prior.RemoteBranch != update.RemoteBranch || prior.RemoteHeadCommit == "" {
		return
	}
	update.RemoteHeadCommit = prior.RemoteHeadCommit
	update.RemoteAhead = prior.RemoteAhead
	update.RemoteBehind = prior.RemoteBehind
}

// parseGitStatusOutput collects tracked status and eligible untracked paths,
// then populates the file lists and map.
func (wt *WorkspaceTracker) parseGitStatusOutput(ctx context.Context, update *types.GitStatusUpdate) error {
	class := gitWorkClass(ctx)
	indexSnapshot, cleanup, err := snapshotGitIndex(ctx, wt.gitIndexPath)
	if err != nil {
		return err
	}
	defer cleanup()
	statusCtx := withGitIndexFile(ctx, indexSnapshot)

	// The tracked query must not receive the dependency-tree exclusion: tracked
	// paths below node_modules remain part of the workspace status. The
	// lockless read and carried observation class preserve the existing Git
	// admission and index-lock behavior.
	statusOut, err := wt.runGitOutputClass(
		statusCtx,
		class,
		true,
		"status", "--porcelain", "--untracked-files=no",
	)
	if err != nil {
		return err
	}

	if err := wt.applyPorcelainOutput(ctx, statusOut, update); err != nil {
		return err
	}

	if wt.gitStatusBetweenQueries != nil {
		wt.gitStatusBetweenQueries()
	}

	untrackedOut, err := wt.runGitOutputClass(statusCtx, class, true, gitUntrackedFilesArgs...)
	if err != nil {
		return err
	}
	return wt.applyUntrackedOutput(ctx, untrackedOut, update)
}

func (wt *WorkspaceTracker) applyPorcelainOutput(
	ctx context.Context,
	statusOut []byte,
	update *types.GitStatusUpdate,
) error {
	// Git status --porcelain format: XY filename
	// X = index (staged) status, Y = working tree (unstaged) status
	// ' ' = unmodified, M = modified, A = added, D = deleted, R = renamed, ? = untracked
	lines := strings.Split(string(statusOut), "\n")
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(line) < 3 {
			continue
		}
		wt.applyPorcelainLine(line, update)
	}
	return nil
}

// unquoteGitPath strips git's C-style quoting from paths that contain spaces or
// special characters. Git wraps such paths in double quotes in porcelain output
// (e.g. "path with spaces/file.md"). Go's strconv.Unquote handles the same
// escaping rules (backslash sequences for tabs, newlines, quotes, etc.).
func unquoteGitPath(p string) string {
	if len(p) >= 2 && p[0] == '"' && p[len(p)-1] == '"' {
		if unquoted, err := strconv.Unquote(p); err == nil {
			return unquoted
		}
	}
	return p
}

// applyPorcelainLine parses a single git status --porcelain line and updates the status update.
func (wt *WorkspaceTracker) applyPorcelainLine(line string, update *types.GitStatusUpdate) {
	indexStatus := line[0]    // Staged status (X)
	workTreeStatus := line[1] // Unstaged status (Y)
	rawPath := strings.TrimSpace(line[3:])

	// For renames the format is "old -> new" (each part may be independently
	// quoted), so we must split first and unquote each part separately.
	filePath := unquoteGitPath(rawPath)
	oldPath := ""
	if indexStatus == 'R' {
		if idx := strings.Index(rawPath, " -> "); idx != -1 {
			oldPath = unquoteGitPath(rawPath[:idx])
			filePath = unquoteGitPath(rawPath[idx+4:])
		}
	}

	fileInfo := types.FileInfo{Path: filePath, OldPath: oldPath}

	// Determine staged status based on index and worktree status.
	// Prioritize worktree changes as they represent the current state.
	switch {
	case indexStatus == '?' && workTreeStatus == '?':
		fileInfo.Status = fileStatusUntracked
		fileInfo.Staged = false
		update.Untracked = append(update.Untracked, filePath)
	case workTreeStatus == 'D':
		// File deleted in worktree - this is an unstaged deletion
		fileInfo.Status = fileStatusDeleted
		fileInfo.Staged = false
		update.Deleted = append(update.Deleted, filePath)
	case indexStatus == 'D':
		// File deleted and staged
		fileInfo.Status = fileStatusDeleted
		fileInfo.Staged = true
		update.Deleted = append(update.Deleted, filePath)
	case workTreeStatus == 'M':
		// Modified in worktree - unstaged modification
		fileInfo.Status = fileStatusModified
		fileInfo.Staged = false
		update.Modified = append(update.Modified, filePath)
	case indexStatus == 'M':
		// Modified and staged (no worktree changes)
		fileInfo.Status = fileStatusModified
		fileInfo.Staged = true
		update.Modified = append(update.Modified, filePath)
	case indexStatus == 'A':
		// Added and staged
		fileInfo.Status = "added"
		fileInfo.Staged = true
		update.Added = append(update.Added, filePath)
	case indexStatus == 'R':
		fileInfo.Status = "renamed"
		fileInfo.Staged = true
		update.Renamed = append(update.Renamed, filePath)
	}

	// A path can have both an index and a working-tree change (for example MM
	// or AM). Keep the flattened compatibility projection above, but preserve
	// both independently for consumers that understand mixed paths.
	if stagedChange := porcelainChangeFacet(indexStatus, oldPath); stagedChange != nil {
		if unstagedChange := porcelainChangeFacet(workTreeStatus, ""); unstagedChange != nil {
			fileInfo.StagedChange = stagedChange
			fileInfo.UnstagedChange = unstagedChange
		}
	}

	update.Files[filePath] = fileInfo
}

func porcelainChangeFacet(status byte, oldPath string) *types.FileChangeFacet {
	change := &types.FileChangeFacet{OldPath: oldPath}
	switch status {
	case 'M':
		change.Status = fileStatusModified
	case 'A':
		change.Status = "added"
	case 'D':
		change.Status = fileStatusDeleted
	case 'R':
		change.Status = "renamed"
	default:
		return nil
	}
	return change
}
