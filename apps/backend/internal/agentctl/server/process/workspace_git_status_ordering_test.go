package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/subproc"
)

func TestWorkspaceTrackerDetailsWaitWithNoLiveJobReturnsUnavailable(t *testing.T) {
	tracker := newStatusConcurrencyTracker(t)
	tracker.mu.Lock()
	tracker.currentStatus = types.GitStatusUpdate{
		Timestamp: time.Now(), TrackerEpoch: tracker.gitStatusEpoch, SnapshotRevision: 1,
		StatusState: gitStatusStateReady, FilesComplete: true, DetailState: gitStatusDetailPending,
		Files: map[string]types.FileInfo{"pending.txt": {Path: "pending.txt"}},
	}
	tracker.gitStatusFingerprint = "pending-without-job"
	tracker.mu.Unlock()
	status, err := tracker.GetGitStatusWithDetails(context.Background(), false)
	if !errors.Is(err, errGitStatusDetailsUnavailable) {
		t.Fatalf("details wait error = %v, want details unavailable", err)
	}
	if status.DetailState != gitStatusDetailPending {
		t.Fatalf("stored snapshot was mutated by failed waiter: %+v", status)
	}
}

func TestWorkspaceTrackerReplayReturnsCacheWithoutStartingObservation(t *testing.T) {
	tracker := newStatusConcurrencyTracker(t)
	accepted := types.GitStatusUpdate{
		Timestamp: time.Unix(9, 0), TrackerEpoch: tracker.gitStatusEpoch, SnapshotRevision: 4,
		StatusState: gitStatusStateReady, FilesComplete: true, DetailState: gitStatusDetailPending,
		Branch: "cached-branch", Files: map[string]types.FileInfo{"cached.txt": {Path: "cached.txt"}},
	}
	tracker.mu.Lock()
	tracker.currentStatus = cloneGitStatusUpdate(accepted)
	tracker.mu.Unlock()
	var observations atomic.Int32
	tracker.gitStatusBasicObserver = func(context.Context) (types.GitStatusUpdate, error) {
		observations.Add(1)
		return types.GitStatusUpdate{StatusState: gitStatusStateReady}, nil
	}

	replayed, err := tracker.GetGitStatusReplay(context.Background())
	if err != nil {
		t.Fatalf("replay status error = %v", err)
	}
	if replayed.SnapshotRevision != accepted.SnapshotRevision || replayed.Timestamp != accepted.Timestamp || replayed.Branch != accepted.Branch {
		t.Fatalf("replayed status = %+v, want the accepted cache %+v", replayed, accepted)
	}
	if got := observations.Load(); got != 0 {
		t.Fatalf("replay started %d observations, want zero", got)
	}
}

func TestWorkspaceTrackerOlderObservationCannotReplaceNewer(t *testing.T) {
	tracker := newStatusConcurrencyTracker(t)
	started := make(chan struct{})
	release := make(chan struct{})
	olderResult := make(chan types.GitStatusUpdate, 1)
	olderErr := make(chan error, 1)
	observer := func(ctx context.Context) (types.GitStatusUpdate, error) {
		if gitWorkClass(ctx) == subproc.GitBackground {
			close(started)
			<-release
			return types.GitStatusUpdate{Timestamp: time.Unix(1, 0), Branch: "older"}, nil
		}
		return types.GitStatusUpdate{Timestamp: time.Unix(2, 0), Branch: "newer"}, nil
	}
	go func() {
		status, err := tracker.observeGitStatusClass(context.Background(), subproc.GitBackground, "basic", observer, true)
		olderResult <- status
		olderErr <- err
	}()
	waitForSignal(t, started, "older observation")
	newer, err := tracker.observeGitStatusClass(context.Background(), subproc.GitInteractive, "basic", observer, true)
	if err != nil {
		t.Fatalf("newer observation error = %v", err)
	}
	close(release)
	if err := <-olderErr; err != nil {
		t.Fatalf("older observation error = %v", err)
	}
	if got := <-olderResult; got.Branch != "newer" || got.SnapshotRevision != newer.SnapshotRevision {
		t.Fatalf("older completion returned %+v, want current accepted snapshot %+v", got, newer)
	}
	tracker.mu.RLock()
	current := cloneGitStatusUpdate(tracker.currentStatus)
	tracker.mu.RUnlock()
	if current.Branch != "newer" || current.SnapshotRevision != newer.SnapshotRevision {
		t.Fatalf("current snapshot = %+v, want newer %+v", current, newer)
	}
}

func TestWorkspaceTrackerLifetimeIDsDisambiguateCollidingLocalEpochs(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	first := NewWorkspaceTracker(repoDir, newTestLogger(t))
	second := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(first.Stop)
	t.Cleanup(second.Stop)
	first.gitStatusEpoch = 1
	second.gitStatusEpoch = 1
	base := types.GitStatusUpdate{
		Timestamp: time.Now(), StatusState: gitStatusStateReady, FilesComplete: true,
		DetailState: gitStatusDetailPending, Files: map[string]types.FileInfo{},
	}
	firstStatus, firstPublished := first.publishGitStatus(base, 1, "same")
	secondStatus, secondPublished := second.publishGitStatus(base, 1, "same")
	if !firstPublished || !secondPublished {
		t.Fatal("fresh tracker status was not published")
	}
	if firstStatus.TrackerEpoch != secondStatus.TrackerEpoch {
		t.Fatalf("test setup did not collide local epochs: %d and %d", firstStatus.TrackerEpoch, secondStatus.TrackerEpoch)
	}
	if firstStatus.TrackerID == "" || secondStatus.TrackerID == "" || firstStatus.TrackerID == secondStatus.TrackerID {
		t.Fatalf("tracker lifetime IDs = %q / %q, want distinct non-empty identities", firstStatus.TrackerID, secondStatus.TrackerID)
	}
}

func TestWorkspaceTrackerRejectsChangedEnrichment(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	path := filepath.Join(repoDir, "README.md")
	writeFile(t, repoDir, "README.md", "old version\n")
	initialInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	subscriber := make(types.WorkspaceStreamSubscriber, 8)
	tracker.workspaceSubMu.Lock()
	tracker.workspaceStreamSubscribers[subscriber] = struct{}{}
	tracker.workspaceSubMu.Unlock()
	t.Cleanup(func() { tracker.DetachWorkspaceStreamSubscriber(subscriber) })
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseSecond := make(chan struct{})
	var releasedFirst, releasedSecond bool
	defer func() {
		if !releasedFirst {
			close(releaseFirst)
		}
		if !releasedSecond {
			close(releaseSecond)
		}
	}()
	var enrichmentCount int
	tracker.gitStatusBeforeEnrich = func() {
		enrichmentCount++
		if enrichmentCount == 1 {
			close(firstStarted)
			<-releaseFirst
			return
		}
		if enrichmentCount == 2 {
			close(secondStarted)
			<-releaseSecond
		}
	}

	initial, err := tracker.GetGitStatus(context.Background(), true)
	if err != nil {
		t.Fatalf("initial status error = %v", err)
	}
	waitForSignal(t, firstStarted, "first enrichment")
	writeFile(t, repoDir, "README.md", "new version\n")
	if err := os.Chtimes(path, initialInfo.ModTime(), initialInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	changed, err := tracker.GetGitStatus(context.Background(), true)
	if err != nil {
		t.Fatalf("changed status error = %v", err)
	}
	if changed.SnapshotRevision <= initial.SnapshotRevision {
		t.Fatalf("changed revision = %d, want after initial %d", changed.SnapshotRevision, initial.SnapshotRevision)
	}
	close(releaseFirst)
	releasedFirst = true
	waitForSignal(t, secondStarted, "corrective enrichment")
	tracker.mu.RLock()
	current := cloneGitStatusUpdate(tracker.currentStatus)
	tracker.mu.RUnlock()
	if current.DetailState != gitStatusDetailPending {
		t.Fatalf("changed capture detail state = %q, want pending until its own enrichment", current.DetailState)
	}
	close(releaseSecond)
	releasedSecond = true
	deadline := time.After(3 * time.Second)
	for ready := false; !ready; {
		select {
		case <-deadline:
			tracker.mu.RLock()
			current = cloneGitStatusUpdate(tracker.currentStatus)
			tracker.mu.RUnlock()
			t.Fatalf("corrective enrichment did not publish: %+v", current)
		case message := <-subscriber:
			if message.GitStatus != nil && message.GitStatus.DetailState == gitStatusDetailReady {
				current = cloneGitStatusUpdate(*message.GitStatus)
				ready = true
			}
		}
	}
	if !strings.Contains(current.Files["README.md"].Diff, "new version") || strings.Contains(current.Files["README.md"].Diff, "old version") {
		t.Fatalf("enriched diff does not match the validated file contents: %q", current.Files["README.md"].Diff)
	}
}

func TestWorkspaceTrackerStopDrainsPendingEnrichmentIndexes(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	writeFile(t, repoDir, "README.md", "first queued state\n")
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	started := make(chan struct{})
	release := make(chan struct{})
	var released bool
	defer func() {
		if !released {
			close(release)
		}
	}()
	tracker.gitStatusBeforeEnrich = func() {
		close(started)
		<-release
	}
	if _, err := tracker.GetGitStatus(context.Background(), true); err != nil {
		t.Fatalf("first status error = %v", err)
	}
	waitForSignal(t, started, "running enrichment")
	writeFile(t, repoDir, "README.md", "second queued state\n")
	if _, err := tracker.GetGitStatus(context.Background(), true); err != nil {
		t.Fatalf("second status error = %v", err)
	}
	writeFile(t, repoDir, "README.md", "latest queued state\n")
	if _, err := tracker.GetGitStatus(context.Background(), true); err != nil {
		t.Fatalf("latest status error = %v", err)
	}
	indexes, err := filepath.Glob(filepath.Join(filepath.Dir(tracker.gitIndexPath), ".kandev-index-snapshot-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 2 {
		t.Fatalf("retained indexes = %d, want running and latest pending jobs: %v", len(indexes), indexes)
	}
	stopped := make(chan struct{})
	go func() {
		tracker.Stop()
		close(stopped)
	}()
	<-tracker.cancelCtx.Done()
	close(release)
	released = true
	waitForSignal(t, stopped, "tracker stop")
	indexes, err = filepath.Glob(filepath.Join(filepath.Dir(tracker.gitIndexPath), ".kandev-index-snapshot-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 0 {
		t.Fatalf("retained index snapshots after Stop = %v", indexes)
	}
}

// TestWorkspaceTrackerExpiredCallerStillPublishes covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.2 and .20.
func TestWorkspaceTrackerExpiredCallerStillPublishes(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	writeFile(t, repoDir, "README.md", "caller timed out\n")

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	started := make(chan struct{})
	release := make(chan struct{})
	var released bool
	defer func() {
		if !released {
			close(release)
		}
	}()
	tracker.gitStatusBetweenQueries = func(context.Context) {
		close(started)
		<-release
	}

	sub := make(types.WorkspaceStreamSubscriber, 8)
	tracker.workspaceSubMu.Lock()
	tracker.workspaceStreamSubscribers[sub] = struct{}{}
	tracker.workspaceSubMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := tracker.GetGitStatus(ctx, true)
		result <- err
	}()
	waitForSignal(t, started, "basic status observation barrier")
	cancel()
	if err := <-result; err != context.Canceled {
		t.Fatalf("caller error = %v, want context.Canceled", err)
	}

	close(release)
	released = true
	select {
	case message := <-sub:
		if message.GitStatus == nil || message.GitStatus.StatusState != "ready" || !message.GitStatus.FilesComplete {
			t.Fatalf("published status = %+v, want complete ready membership", message.GitStatus)
		}
		if _, ok := message.GitStatus.Files["README.md"]; !ok {
			t.Fatalf("published status is missing the changed file: %+v", message.GitStatus.Files)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("accepted shared status was not published after its caller stopped waiting")
	}

	tracker.mu.RLock()
	cached := tracker.currentStatus
	tracker.mu.RUnlock()
	if cached.Timestamp.IsZero() || cached.Files["README.md"].Path != "README.md" {
		t.Fatalf("cached status = %+v, want the accepted complete snapshot", cached)
	}
}
