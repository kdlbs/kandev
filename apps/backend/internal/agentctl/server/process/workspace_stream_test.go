package process

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// TestAttachWorkspaceStreamSubscriber_ReplaysAcceptedSnapshot verifies that a
// stream attach replays the tracker-owned snapshot without creating a second
// status observation or changing its ordering token.
func TestAttachWorkspaceStreamSubscriber_ReplaysAcceptedSnapshot(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(wt.Stop)
	accepted, err := wt.GetGitStatus(context.Background(), true)
	if err != nil {
		t.Fatalf("GetGitStatus() error = %v", err)
	}

	sub := make(types.WorkspaceStreamSubscriber, 4)
	wt.AttachWorkspaceStreamSubscriber(sub)
	defer wt.DetachWorkspaceStreamSubscriber(sub)

	select {
	case msg := <-sub:
		if msg.GitStatus == nil {
			t.Fatalf("expected GitStatus message on attach, got %+v", msg)
		}
		if msg.GitStatus.SnapshotRevision != accepted.SnapshotRevision || msg.GitStatus.TrackerEpoch != accepted.TrackerEpoch {
			t.Errorf("replay ordering = (%d, %d), want accepted (%d, %d)", msg.GitStatus.TrackerEpoch, msg.GitStatus.SnapshotRevision, accepted.TrackerEpoch, accepted.SnapshotRevision)
		}
		if !msg.GitStatus.FilesComplete || msg.GitStatus.StatusState != gitStatusStateReady {
			t.Errorf("replayed status quality = (%q, %v), want ready complete membership", msg.GitStatus.StatusState, msg.GitStatus.FilesComplete)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for replayed status on subscribe")
	}
}

// TestAttachWorkspaceStreamSubscriber_BareTrackerSkipsReplay guards against
// the refresh hook accidentally running git status against a non-repo
// directory (multi-repo task root). The bare tracker has gitIndexPath="" and
// must skip both the refresh and the replay.
func TestAttachWorkspaceStreamSubscriber_BareTrackerSkipsReplay(t *testing.T) {
	isolateTestGitEnv(t)

	dir := t.TempDir()
	log := newTestLogger(t)
	wt := NewWorkspaceTracker(dir, log)
	// Force bare-tracker behaviour even if NewWorkspaceTracker happened to
	// find a git index (unlikely in a fresh TempDir, but be explicit).
	wt.gitIndexPath = ""

	sub := make(types.WorkspaceStreamSubscriber, 4)
	wt.AttachWorkspaceStreamSubscriber(sub)
	defer wt.DetachWorkspaceStreamSubscriber(sub)

	select {
	case msg := <-sub:
		t.Fatalf("bare tracker should not send anything on attach, got %+v", msg)
	case <-time.After(100 * time.Millisecond):
		// Good — no message sent.
	}
}

// TestGitOperator_Commit_RefreshesGitStatus verifies that GitOperator.Commit
// refreshes currentStatus after the commit completes, matching the
// Stage/Unstage/Discard/Push pattern. Without this, the UI's "unstaged" list
// keeps the pre-commit entries until the next poll tick — which never fires
// in PollModePaused.
func TestGitOperator_Commit_RefreshesGitStatus(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	// Modify and stage a tracked file, then warm the cache so it shows the
	// pre-commit "modified=1" state.
	writeFile(t, repoDir, "README.md", "# Modified\n")
	runGit(t, repoDir, "add", "README.md")
	wt.updateGitStatus(ctx)

	wt.mu.RLock()
	preFiles := len(wt.currentStatus.Files)
	wt.mu.RUnlock()
	if preFiles == 0 {
		t.Fatalf("test setup: expected cached status to show 1+ file pre-commit, got 0")
	}

	gitOp := NewGitOperator(repoDir, log, wt)
	result, err := gitOp.Commit(ctx, "test: refresh after commit", false, false)
	if err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Commit failed: %+v", result)
	}

	// Post-commit the working tree is clean; currentStatus must reflect that.
	wt.mu.RLock()
	gotFiles := len(wt.currentStatus.Files)
	gotModified := append([]string(nil), wt.currentStatus.Modified...)
	wt.mu.RUnlock()

	if gotFiles != 0 {
		t.Errorf("expected currentStatus.Files=0 after commit, got %d", gotFiles)
	}
	if len(gotModified) != 0 {
		t.Errorf("expected currentStatus.Modified=[] after commit, got %v", gotModified)
	}
}

// TestGitOperator_Commit_BroadcastsFreshStatus verifies that an already-attached
// stream subscriber receives a fresh status_update after a commit. This is
// the end-to-end path the orchestrator + frontend rely on: poll-driven
// detection of an agent's commit goes through this broadcast.
func TestGitOperator_Commit_BroadcastsFreshStatus(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	writeFile(t, repoDir, "README.md", "# Modified\n")
	runGit(t, repoDir, "add", "README.md")
	wt.updateGitStatus(ctx)

	sub := make(types.WorkspaceStreamSubscriber, 8)
	wt.AttachWorkspaceStreamSubscriber(sub)
	defer wt.DetachWorkspaceStreamSubscriber(sub)

	// Drain the on-attach replay so we observe only commit-driven messages.
	drain(t, sub)

	gitOp := NewGitOperator(repoDir, log, wt)
	if _, err := gitOp.Commit(ctx, "test: broadcast after commit", false, false); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Expect at least one git_status broadcast carrying a clean working tree.
	// A commit notification may also be delivered; ignore it for this test.
	sawCleanStatus := false
	deadline := time.After(2 * time.Second)
	for !sawCleanStatus {
		select {
		case msg := <-sub:
			if msg.GitStatus == nil {
				continue
			}
			if len(msg.GitStatus.Modified) == 0 && len(msg.GitStatus.Files) == 0 {
				sawCleanStatus = true
			}
		case <-deadline:
			t.Fatal("did not observe a clean status broadcast after commit")
		}
	}
}

// TestAttachWorkspaceStreamSubscriber_DoesNotBroadcastToOthers guards against
// regressions where the refresh-before-replay hook accidentally goes through
// notifyWorkspaceStreamGitStatus. That would noise up every already-attached
// subscriber on every new attach and double-deliver to the attaching one.
func TestAttachWorkspaceStreamSubscriber_DoesNotBroadcastToOthers(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	t.Cleanup(wt.Stop)
	enrichmentStarted := make(chan struct{})
	releaseEnrichment := make(chan struct{})
	var released bool
	defer func() {
		if !released {
			close(releaseEnrichment)
		}
	}()
	wt.gitStatusBeforeEnrich = func() {
		close(enrichmentStarted)
		<-releaseEnrichment
	}
	if _, err := wt.GetGitStatus(context.Background(), true); err != nil {
		t.Fatalf("GetGitStatus() error = %v", err)
	}
	waitForSignal(t, enrichmentStarted, "enrichment gate")

	existing := make(types.WorkspaceStreamSubscriber, 8)
	wt.AttachWorkspaceStreamSubscriber(existing)
	defer wt.DetachWorkspaceStreamSubscriber(existing)
	drain(t, existing)

	newSub := make(types.WorkspaceStreamSubscriber, 8)
	wt.AttachWorkspaceStreamSubscriber(newSub)
	defer wt.DetachWorkspaceStreamSubscriber(newSub)

	// The newly attached subscriber must receive exactly one replay frame.
	select {
	case msg := <-newSub:
		if msg.GitStatus == nil {
			t.Fatalf("expected GitStatus replay, got %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("new subscriber did not receive the replay")
	}
	select {
	case msg := <-newSub:
		t.Fatalf("new subscriber received a second frame, want one: %+v", msg)
	case <-time.After(150 * time.Millisecond):
		// Good — only one frame delivered.
	}

	// The pre-existing subscriber must receive nothing on this attach.
	select {
	case msg := <-existing:
		t.Fatalf("existing subscriber should not receive a frame when another attaches, got %+v", msg)
	case <-time.After(150 * time.Millisecond):
		// Good — no noise.
	}
	close(releaseEnrichment)
	released = true
}

func drain(t *testing.T, sub types.WorkspaceStreamSubscriber) {
	t.Helper()
	for {
		select {
		case <-sub:
		case <-time.After(100 * time.Millisecond):
			return
		}
	}
}
