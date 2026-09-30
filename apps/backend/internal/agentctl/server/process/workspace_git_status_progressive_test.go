package process

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/subproc"
)

// TestWorkspaceTrackerDirtyFilesVisibleBeforeEnrichment covers
// AC-PLATFORM-WORKSPACE-GIT-STATUS-001.1 and .19.
func TestWorkspaceTrackerDirtyFilesVisibleBeforeEnrichment(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	writeFile(t, repoDir, "mixed.txt", "base\n")
	runGit(t, repoDir, "add", "mixed.txt")
	runGit(t, repoDir, "commit", "-m", "Add mixed fixture")
	writeFile(t, repoDir, "mixed.txt", "base\nstaged\n")
	runGit(t, repoDir, "add", "mixed.txt")
	writeFile(t, repoDir, "mixed.txt", "base\nstaged\nunstaged\n")
	writeFile(t, repoDir, "README.md", "unstaged only\n")
	writeFile(t, repoDir, "staged.txt", "staged only\n")
	runGit(t, repoDir, "add", "staged.txt")
	writeFile(t, repoDir, "untracked.txt", "untracked\n")

	tracePath := filepath.Join(t.TempDir(), "git.trace")
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracker.SetGitEnvironment([]string{"GIT_TRACE=" + tracePath})
	t.Cleanup(tracker.Stop)
	enrichmentStarted := make(chan struct{})
	releaseEnrichment := make(chan struct{})
	var released bool
	defer func() {
		if !released {
			close(releaseEnrichment)
		}
	}()
	tracker.gitStatusBeforeEnrich = func() {
		close(enrichmentStarted)
		<-releaseEnrichment
	}

	status, err := tracker.GetGitStatus(context.Background(), true)
	if err != nil {
		t.Fatalf("GetGitStatus() error = %v", err)
	}
	for _, path := range []string{"mixed.txt", "README.md", "staged.txt", "untracked.txt"} {
		if _, ok := status.Files[path]; !ok {
			t.Errorf("basic status is missing dirty path %q", path)
		}
	}
	mixed := status.Files["mixed.txt"]
	if mixed.StagedChange == nil || mixed.UnstagedChange == nil {
		t.Fatalf("mixed status = %+v, want both staged and unstaged facets", mixed)
	}
	waitForSignal(t, enrichmentStarted, "asynchronous enrichment gate")

	wire, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(wire, &encoded); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	assertJSONValue(t, encoded, "status_state", "ready")
	assertJSONValue(t, encoded, "files_complete", true)
	assertJSONValue(t, encoded, "detail_state", "pending")
	assertFileDiffState(t, encoded, "mixed.txt", "diff_state", "pending")
	assertFileDiffState(t, encoded, "mixed.txt", "staged_change", "pending")
	assertFileDiffState(t, encoded, "mixed.txt", "unstaged_change", "pending")

	trace, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read Git trace: %v", err)
	}
	if strings.Contains(string(trace), "git diff ") || strings.Contains(string(trace), "git diff\n") {
		t.Fatalf("basic status ran diff enrichment before returning:\n%s", trace)
	}
	close(releaseEnrichment)
	released = true
}

func TestWorkspaceTrackerUnchangedDirtyReplay(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	writeFile(t, repoDir, "README.md", "dirty replay\n")

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

	first, err := tracker.GetGitStatus(context.Background(), true)
	if err != nil {
		t.Fatalf("fresh status error = %v", err)
	}
	waitForSignal(t, started, "first enrichment")
	replayed, err := tracker.GetGitStatus(context.Background(), false)
	if err != nil {
		t.Fatalf("cached status error = %v", err)
	}
	if replayed.Timestamp != first.Timestamp || replayed.TrackerEpoch != first.TrackerEpoch || replayed.SnapshotRevision != first.SnapshotRevision ||
		replayed.DetailState != gitStatusDetailPending || len(replayed.Files) != 1 {
		t.Fatalf("cached replay = %+v, want unchanged pending snapshot %+v", replayed, first)
	}
	tracker.mu.RLock()
	cached := cloneGitStatusUpdate(tracker.currentStatus)
	tracker.mu.RUnlock()
	if cached.SnapshotRevision != first.SnapshotRevision || cached.Timestamp != first.Timestamp {
		t.Fatalf("cache changed during replay: %+v, first snapshot: %+v", cached, first)
	}
	close(release)
	released = true
}

func TestWorkspaceTrackerEnrichmentValidationUsesBoundedBackgroundAdmission(t *testing.T) {
	restore := subproc.Git().SetCapForTest(1)
	t.Cleanup(restore)

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	writeFile(t, repoDir, "README.md", "pending enrichment\n")
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracker.gitStatusObserveTimeout = 250 * time.Millisecond
	t.Cleanup(tracker.Stop)
	enrichmentStarted := make(chan struct{})
	releaseEnrichment := make(chan struct{})
	var released bool
	defer func() {
		if !released {
			close(releaseEnrichment)
		}
	}()
	tracker.gitStatusBeforeEnrich = func() {
		close(enrichmentStarted)
		<-releaseEnrichment
	}

	status, err := tracker.GetGitStatus(context.Background(), true)
	if err != nil {
		t.Fatalf("fresh status: %v", err)
	}
	if status.DetailState != gitStatusDetailPending {
		t.Fatalf("basic detail state = %q, want pending", status.DetailState)
	}
	waitForSignal(t, enrichmentStarted, "enrichment validator gate")
	held, err := subproc.AcquireGit(context.Background(), subproc.GitBackground)
	if err != nil {
		t.Fatalf("hold background Git slot: %v", err)
	}
	defer held()
	close(releaseEnrichment)
	released = true

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && subproc.AdmissionSnapshot().Classes[string(subproc.GitBackground)].Waiters == 0 {
		runtime.Gosched()
	}
	if got := subproc.AdmissionSnapshot().Classes[string(subproc.GitBackground)].Waiters; got != 1 {
		t.Fatalf("background admission waiters = %d, want validator queued in background class", got)
	}

	tracker.gitStatusEnrichmentMu.Lock()
	job := tracker.gitStatusEnrichmentJob
	tracker.gitStatusEnrichmentMu.Unlock()
	if job == nil {
		t.Fatal("tracker has no active enrichment job")
	}
	waitForSignal(t, job.done, "bounded validation deadline")
	if !errors.Is(job.err, context.DeadlineExceeded) {
		t.Fatalf("enrichment error = %v, want deadline exceeded", job.err)
	}
	if _, err := os.Stat(job.indexSnapshot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pinned index still exists after deadline: stat error = %v", err)
	}
}

func TestWorkspaceTrackerLargeAndAggregateSourcesDoNotConsumeDiffOutputBudgetAsEvidenceBudget(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	largeContent := strings.Repeat("line of tracked content\n", 140_000)
	writeFile(t, repoDir, "large.txt", largeContent)
	mediumContent := strings.Repeat("small tracked content\n", 14_000)
	const aggregateFileCount = 8
	aggregatePaths := make([]string, 0, aggregateFileCount)
	for index := 0; index < aggregateFileCount; index++ {
		path := fmt.Sprintf("aggregate-%02d.txt", index)
		aggregatePaths = append(aggregatePaths, path)
		writeFile(t, repoDir, path, mediumContent)
	}
	runGit(t, repoDir, append([]string{"add", "large.txt"}, aggregatePaths...)...)
	runGit(t, repoDir, "commit", "-m", "Add changed tracked files")
	runGit(t, repoDir, "update-ref", "refs/remotes/origin/main", "HEAD")
	writeFile(t, repoDir, "large.txt", largeContent+"small edit\n")
	writeFile(t, repoDir, "small.txt", "small change\n")
	for _, path := range aggregatePaths {
		writeFile(t, repoDir, path, mediumContent+"tiny edit\n")
	}

	log, observed := newObservedTestLogger(t)
	tracker := NewWorkspaceTracker(repoDir, log)
	t.Cleanup(tracker.Stop)
	capture, err := tracker.captureBasicGitStatus(context.Background())
	if err != nil {
		t.Fatalf("basic capture: %v", err)
	}
	if capture.job == nil {
		t.Fatal("basic capture did not retain enrichment evidence")
	}
	defer capture.job.indexCleanup()
	accepted, published := tracker.publishGitStatus(capture.status, 1, capture.fingerprint)
	if !published {
		t.Fatal("basic membership was not published")
	}
	capture.job.status = cloneGitStatusUpdate(accepted)
	capture.job.correctionPermitted = false
	if err := tracker.runGitStatusEnrichment(capture.job); err != nil {
		t.Fatalf("enrichment: %v", err)
	}
	tracker.mu.RLock()
	status := cloneGitStatusUpdate(tracker.currentStatus)
	tracker.mu.RUnlock()
	if status.DetailState != gitStatusDetailReady {
		t.Fatalf("enriched status = %+v, logs = %+v, want ready despite raw contents exceeding diff-output budget", status, observed.All())
	}
	assertPaths := append([]string{"large.txt", "small.txt"}, aggregatePaths...)
	for _, path := range assertPaths {
		file, ok := status.Files[path]
		if !ok {
			t.Fatalf("enriched status is missing %q", path)
		}
		if file.DiffState != gitStatusDiffReady {
			t.Errorf("%s diff state = %q, want ready", path, file.DiffState)
		}
	}
}

func TestWorkspaceTrackerRetriesOnlyFailedDiffAfterTransientGitFailure(t *testing.T) {
	gitBinary, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find git: %v", err)
	}
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	writeFile(t, repoDir, "broken.txt", "base broken\n")
	writeFile(t, repoDir, "healthy.txt", "base healthy\n")
	runGit(t, repoDir, "add", "broken.txt", "healthy.txt")
	runGit(t, repoDir, "commit", "-m", "Add changed files")
	runGit(t, repoDir, "update-ref", "refs/remotes/origin/main", "HEAD")
	writeFile(t, repoDir, "broken.txt", "base broken\nchanged broken\n")
	writeFile(t, repoDir, "healthy.txt", "base healthy\nchanged healthy\n")

	binDir := t.TempDir()
	gitShim := filepath.Join(binDir, "git")
	shim := "#!/bin/sh\ncase \"$*\" in\n  *\"-- broken.txt\"*) exit 41 ;;\nesac\nexec " + gitBinary + " \"$@\"\n"
	if err := os.WriteFile(gitShim, []byte(shim), 0o700); err != nil {
		t.Fatalf("write Git shim: %v", err)
	}
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	tracker.SetGitEnvironment(prependGitPath(binDir, os.Environ()))

	first, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if !errors.Is(err, errGitStatusDetailsUnavailable) {
		t.Fatalf("first details error = %v, want unavailable after one diff command fails", err)
	}
	if first.DetailState != gitStatusDetailUnavailable {
		t.Fatalf("first detail quality = %q, want unavailable", first.DetailState)
	}
	if first.Files["broken.txt"].DiffState != gitStatusDiffUnavailable {
		t.Fatalf("failed file state = %q, want unavailable", first.Files["broken.txt"].DiffState)
	}
	if first.Files["healthy.txt"].DiffState != gitStatusDiffReady || !strings.Contains(first.Files["healthy.txt"].Diff, "changed healthy") {
		t.Fatalf("healthy file details were lost after sibling command failure: %+v", first.Files["healthy.txt"])
	}

	t.Setenv("PATH", originalPath)
	tracker.SetGitEnvironment(os.Environ())
	retried, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if err != nil {
		t.Fatalf("retry on unchanged repository: %v", err)
	}
	if retried.DetailState != gitStatusDetailReady || retried.Files["broken.txt"].DiffState != gitStatusDiffReady ||
		!strings.Contains(retried.Files["broken.txt"].Diff, "changed broken") {
		t.Fatalf("retry did not repair failed details: %+v", retried)
	}
}

func prependGitPath(binDir string, env []string) []string {
	result := make([]string, 0, len(env)+1)
	for _, value := range env {
		if !strings.HasPrefix(value, "PATH=") {
			result = append(result, value)
		}
	}
	return append(result, "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestWorkspaceTrackerEnrichmentDeduplicatesUnchangedCapture(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	writeFile(t, repoDir, "README.md", "deduplicate\n")
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
	var starts atomic.Int32
	tracker.gitStatusBeforeEnrich = func() {
		if starts.Add(1) == 1 {
			close(started)
			<-release
		}
	}

	first, err := tracker.GetGitStatus(context.Background(), true)
	if err != nil {
		t.Fatalf("first status error = %v", err)
	}
	waitForSignal(t, started, "first enrichment")
	second, err := tracker.GetGitStatus(context.Background(), true)
	if err != nil {
		t.Fatalf("second status error = %v", err)
	}
	if second.SnapshotRevision != first.SnapshotRevision || second.DetailState != gitStatusDetailPending {
		t.Fatalf("unchanged fresh status = %+v, first = %+v", second, first)
	}
	indexes, err := filepath.Glob(filepath.Join(filepath.Dir(tracker.gitIndexPath), ".kandev-index-snapshot-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 1 {
		t.Fatalf("retained index snapshots = %d, want one deduplicated worker: %v", len(indexes), indexes)
	}
	close(release)
	released = true
}

func TestWorkspaceTrackerDetailsWaitCanCancelWithoutCancelingEnrichment(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	writeFile(t, repoDir, "README.md", "details wait\n")

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

	initial, err := tracker.GetGitStatus(context.Background(), true)
	if err != nil {
		t.Fatalf("basic status error = %v", err)
	}
	waitForSignal(t, started, "enrichment start")

	waitCtx, cancelWait := context.WithCancel(context.Background())
	waitResult := make(chan error, 1)
	waitJoined := make(chan struct{})
	tracker.gitStatusDetailsWaitJoined = func() { close(waitJoined) }
	go func() {
		_, err := tracker.GetGitStatusWithDetails(waitCtx, false)
		waitResult <- err
	}()
	waitForSignal(t, waitJoined, "detail waiter")
	cancelWait()
	select {
	case err := <-waitResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("details wait error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled detail waiter did not return")
	}

	close(release)
	released = true
	deadline := time.After(3 * time.Second)
	for {
		status, err := tracker.GetGitStatus(context.Background(), false)
		if err != nil {
			t.Fatalf("cached status error = %v", err)
		}
		if status.DetailState == gitStatusDetailReady {
			if status.TrackerEpoch != initial.TrackerEpoch || status.SnapshotRevision <= initial.SnapshotRevision {
				t.Fatalf("completed status = %+v, want same epoch and a later revision than %+v", status, initial)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatalf("enrichment did not complete after waiter cancellation: %+v", status)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestWorkspaceTrackerDetailsWaitRejectsSupersededSnapshot(t *testing.T) {
	tracker := newStatusConcurrencyTracker(t)
	oldStatus := types.GitStatusUpdate{
		Timestamp: time.Unix(1, 0), TrackerEpoch: tracker.gitStatusEpoch, SnapshotRevision: 1,
		StatusState: gitStatusStateReady, FilesComplete: true, DetailState: gitStatusDetailPending,
		Files: map[string]types.FileInfo{"old.txt": {Path: "old.txt"}},
	}
	newStatus := types.GitStatusUpdate{
		Timestamp: time.Unix(2, 0), TrackerEpoch: tracker.gitStatusEpoch, SnapshotRevision: 2,
		StatusState: gitStatusStateReady, FilesComplete: true, DetailState: gitStatusDetailPending,
		Files: map[string]types.FileInfo{"new.txt": {Path: "new.txt"}},
	}
	job := &gitStatusEnrichmentJob{fingerprint: "old", done: make(chan struct{})}
	tracker.mu.Lock()
	tracker.currentStatus = cloneGitStatusUpdate(oldStatus)
	tracker.gitStatusFingerprint = "old"
	tracker.mu.Unlock()
	tracker.gitStatusEnrichmentMu.Lock()
	tracker.gitStatusEnrichmentJob = job
	tracker.gitStatusEnrichmentMu.Unlock()
	joined := make(chan struct{})
	tracker.gitStatusDetailsWaitJoined = func() { close(joined) }
	resultCh := make(chan types.GitStatusUpdate, 1)
	errCh := make(chan error, 1)
	go func() {
		status, err := tracker.GetGitStatusWithDetails(context.Background(), false)
		resultCh <- status
		errCh <- err
	}()
	waitForSignal(t, joined, "old detail waiter")
	tracker.mu.Lock()
	tracker.currentStatus = cloneGitStatusUpdate(newStatus)
	tracker.gitStatusFingerprint = "new"
	tracker.mu.Unlock()
	tracker.gitStatusEnrichmentMu.Lock()
	completeGitStatusEnrichmentLocked(job, errGitStatusEvidenceChanged)
	tracker.gitStatusEnrichmentMu.Unlock()

	if err := <-errCh; !errors.Is(err, errGitStatusEvidenceChanged) {
		t.Fatalf("superseded waiter error = %v, want evidence-changed", err)
	}
	status := <-resultCh
	if status.SnapshotRevision != newStatus.SnapshotRevision || len(status.Files) != 1 {
		t.Fatalf("superseded waiter returned %+v, want only the newer basic snapshot", status)
	}
	if _, ok := status.Files["old.txt"]; ok {
		t.Fatalf("superseded waiter returned old snapshot files: %#v", status.Files)
	}
}

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
	tracker.gitStatusBetweenQueries = func() {
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

func assertJSONValue(t *testing.T, object map[string]json.RawMessage, key string, want any) {
	t.Helper()
	value, ok := object[key]
	if !ok {
		t.Fatalf("status is missing %q: %v", key, object)
	}
	var got any
	if err := json.Unmarshal(value, &got); err != nil {
		t.Fatalf("decode %q: %v", key, err)
	}
	if got != want {
		t.Errorf("%s = %v, want %v", key, got, want)
	}
}

func assertFileDiffState(t *testing.T, status map[string]json.RawMessage, path, key, want string) {
	t.Helper()
	var files map[string]json.RawMessage
	if err := json.Unmarshal(status["files"], &files); err != nil {
		t.Fatalf("decode files: %v", err)
	}
	var file map[string]json.RawMessage
	if err := json.Unmarshal(files[path], &file); err != nil {
		t.Fatalf("decode file %q: %v", path, err)
	}
	if facet := file[key]; key == "staged_change" || key == "unstaged_change" {
		var value map[string]json.RawMessage
		if err := json.Unmarshal(facet, &value); err != nil {
			t.Fatalf("decode %s for %q: %v", key, path, err)
		}
		file = value
		key = "diff_state"
	}
	var got string
	if err := json.Unmarshal(file[key], &got); err != nil {
		t.Fatalf("decode %s for %q: %v", key, path, err)
	}
	if got != want {
		t.Errorf("%s.%s = %q, want %q", path, key, got, want)
	}
}
