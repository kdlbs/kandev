package process

import (
	"context"
	"testing"
)

// TestCheckGitChanges_DetectsPushWithNoOtherChange pins the structural fix
// for a production bug: pollGitChanges' checkGitChanges only re-published
// git status on a HEAD, branch, or working-tree-index change. A push moves
// none of those three — the upstream ref is the only thing that changes —
// so a push-only event was previously invisible to change detection and
// tryUpdateGitStatus never re-fired, meaning the refreshed status carrying
// the new RemoteAhead/RemoteBranch values (which push-detection in
// event_handlers_git.go depends on) was never published at all. This test
// primes the tracker's cache to a pre-push state, pushes for real, and
// asserts a single checkGitChanges call republishes status with the
// post-push RemoteAhead/RemoteBranch, on a HEAD that never moves after the
// prime (matching an already-committed, not-yet-pushed branch).
func TestCheckGitChanges_DetectsPushWithNoOtherChange(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	runGit(t, repoDir, "checkout", "-b", "feature/push-only")
	writeFile(t, repoDir, "feature.txt", "local change")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "local change")

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	// Prime the cache exactly as pollGitChanges' startup block does, so this
	// test exercises checkGitChanges' own change-detection rather than the
	// unconditional first-ever-status-computation path.
	primeSnap, err := wt.readGitPollSnapshot(ctx)
	if err != nil {
		t.Fatalf("readGitPollSnapshot failed: %v", err)
	}
	wt.gitStateMu.Lock()
	wt.cachedHeadSHA = primeSnap.headSHA
	wt.cachedBranchName = primeSnap.branch
	wt.cachedIndexHash = primeSnap.indexHash
	wt.cachedUpstreamSHA = primeSnap.upstreamSHA
	wt.gitStateMu.Unlock()
	if wt.cachedUpstreamSHA != "" {
		t.Fatalf("expected no upstream before the push, got %q", wt.cachedUpstreamSHA)
	}

	// A no-op tick: nothing changed since priming, so status must not have
	// been computed yet (currentStatus stays at its zero value).
	noopSnap, err := wt.readGitPollSnapshot(ctx)
	if err != nil {
		t.Fatalf("readGitPollSnapshot failed: %v", err)
	}
	wt.checkGitChanges(ctx, noopSnap)
	wt.mu.RLock()
	stillEmpty := wt.currentStatus.HeadCommit == ""
	wt.mu.RUnlock()
	if !stillEmpty {
		t.Fatal("expected no status update on a tick with nothing changed since priming")
	}

	runGit(t, repoDir, "push", "-u", "origin", "feature/push-only")

	// HEAD, branch name, and the working tree are all unchanged by the push
	// — only the upstream ref moved. This is the exact case that used to be
	// invisible to checkGitChanges.
	pushSnap, err := wt.readGitPollSnapshot(ctx)
	if err != nil {
		t.Fatalf("readGitPollSnapshot failed: %v", err)
	}
	wt.checkGitChanges(ctx, pushSnap)

	wt.mu.RLock()
	status := wt.currentStatus
	wt.mu.RUnlock()
	if status.RemoteBranch == "" {
		t.Fatal("expected RemoteBranch to be populated after checkGitChanges observed the push")
	}
	if status.RemoteAhead != 0 {
		t.Fatalf("RemoteAhead = %d, want 0 after push", status.RemoteAhead)
	}
}

// TestHandleUpstreamOnlyChange_RetriesWhenUpdateSkipped pins the fix for a
// race in handleUpstreamOnlyChange: if updateMu is held by a concurrent
// RefreshGitStatus (e.g. triggered by a git operation elsewhere) at the exact
// tick that observes an upstream ref move, tryUpdateGitStatus is skipped and
// no refreshed RemoteAhead/RemoteBranch is published. The handler must leave
// cachedUpstreamSHA at its old value in that case, so the next tick's
// compareToCachedState sees the same "changed" delta and retries — instead of
// marking the push observed while nothing was ever published, which would
// permanently starve push detection of the status update it needs.
func TestHandleUpstreamOnlyChange_RetriesWhenUpdateSkipped(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	runGit(t, repoDir, "checkout", "-b", "feature/skipped-update")
	writeFile(t, repoDir, "feature.txt", "local change")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "local change")

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	primeSnap, err := wt.readGitPollSnapshot(ctx)
	if err != nil {
		t.Fatalf("readGitPollSnapshot failed: %v", err)
	}
	wt.gitStateMu.Lock()
	wt.cachedHeadSHA = primeSnap.headSHA
	wt.cachedBranchName = primeSnap.branch
	wt.cachedIndexHash = primeSnap.indexHash
	wt.cachedUpstreamSHA = primeSnap.upstreamSHA
	wt.gitStateMu.Unlock()

	runGit(t, repoDir, "push", "-u", "origin", "feature/skipped-update")
	pushSnap, err := wt.readGitPollSnapshot(ctx)
	if err != nil {
		t.Fatalf("readGitPollSnapshot failed: %v", err)
	}
	if pushSnap.upstreamSHA == "" {
		t.Fatal("expected a non-empty upstream SHA after the push")
	}

	// Simulate a concurrent RefreshGitStatus already in progress.
	wt.updateMu.Lock()
	wt.checkGitChanges(ctx, pushSnap)
	wt.updateMu.Unlock()

	wt.gitStateMu.RLock()
	cachedAfterSkip := wt.cachedUpstreamSHA
	wt.gitStateMu.RUnlock()
	if cachedAfterSkip != "" {
		t.Fatalf("cachedUpstreamSHA = %q, want empty (unchanged) after a skipped update — the push event must not be marked observed", cachedAfterSkip)
	}
	wt.mu.RLock()
	stillEmpty := wt.currentStatus.HeadCommit == ""
	wt.mu.RUnlock()
	if !stillEmpty {
		t.Fatal("expected no status to have been published while updateMu was held")
	}

	// Next tick, lock is free: the same delta must be retried and succeed.
	wt.checkGitChanges(ctx, pushSnap)

	wt.gitStateMu.RLock()
	cachedAfterRetry := wt.cachedUpstreamSHA
	wt.gitStateMu.RUnlock()
	if cachedAfterRetry != pushSnap.upstreamSHA {
		t.Fatalf("cachedUpstreamSHA = %q, want %q after the retried tick", cachedAfterRetry, pushSnap.upstreamSHA)
	}
	wt.mu.RLock()
	status := wt.currentStatus
	wt.mu.RUnlock()
	if status.RemoteBranch == "" {
		t.Fatal("expected RemoteBranch to be populated after the retried tick")
	}
	if status.RemoteAhead != 0 {
		t.Fatalf("RemoteAhead = %d, want 0 after the retried tick", status.RemoteAhead)
	}
}

// TestReadGitPollSnapshot_UpstreamLookupErrorPropagates pins the fix for a
// silent-failure mode in getUpstreamSHA: git can report `branch.upstream` in
// --porcelain=v2 output (from branch.<name>.remote/merge config) while the
// remote-tracking ref itself is gone, so `git rev-parse @{upstream}` fails.
// Coercing that failure to "" would be indistinguishable from "no upstream
// configured" and could mask a real transition; readGitPollSnapshot must
// instead fail the whole snapshot so gitPollTick's existing retry path (via
// handleGitPollFailure) picks it up on the next tick.
func TestReadGitPollSnapshot_UpstreamLookupErrorPropagates(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	runGit(t, repoDir, "checkout", "-b", "feature/dangling-upstream")
	runGit(t, repoDir, "push", "-u", "origin", "feature/dangling-upstream")

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	if _, err := wt.readGitPollSnapshot(ctx); err != nil {
		t.Fatalf("readGitPollSnapshot failed before breaking the upstream ref: %v", err)
	}

	// branch.<name>.remote/merge config stays intact; only the
	// remote-tracking ref is deleted, so git status still reports
	// branch.upstream while `@{upstream}` no longer resolves.
	runGit(t, repoDir, "update-ref", "-d", "refs/remotes/origin/feature/dangling-upstream")

	if _, err := wt.readGitPollSnapshot(ctx); err == nil {
		t.Fatal("expected readGitPollSnapshot to fail when @{upstream} can't be resolved, got nil error")
	}
}
