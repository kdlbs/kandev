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
