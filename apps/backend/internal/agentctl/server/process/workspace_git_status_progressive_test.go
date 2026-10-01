package process

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	tracker.gitStatusEnrichmentTimeout = 250 * time.Millisecond
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

func TestWorkspaceTrackerStopWaitsForCorrectionObservation(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	current := &gitStatusEnrichmentJob{fingerprint: "current", correctionPermitted: true}
	tracker.gitStatusEnrichmentMu.Lock()
	tracker.gitStatusEnrichmentJob = current
	tracker.gitStatusEnrichmentMu.Unlock()
	tracker.gitStatusBeforeCorrection = func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-release
	}
	tracker.requestGitStatusCorrection(current)
	waitForSignal(t, started, "correction observer")
	if !current.correctionRequested {
		t.Fatal("active enrichment was not marked before correction observation")
	}

	stopped := make(chan struct{})
	go func() {
		tracker.Stop()
		close(stopped)
	}()
	waitForSignal(t, canceled, "tracker cancellation")
	select {
	case <-stopped:
		t.Fatal("Stop returned while the correction observation was still gated")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return after the correction observation was released")
	}
}

func TestWorkspaceTrackerQueuesSameFingerprintCorrectionBeforeFailedAttemptSettles(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)

	current := &gitStatusEnrichmentJob{fingerprint: "same", correctionRequested: true}
	replacement := &gitStatusEnrichmentJob{
		status:                  types.GitStatusUpdate{DetailState: gitStatusDetailPending},
		fingerprint:             "same",
		done:                    make(chan struct{}),
		indexCleanup:            func() {},
		contentEvidenceComplete: true,
	}
	tracker.gitStatusEnrichmentMu.Lock()
	tracker.gitStatusEnrichmentRun = true
	tracker.gitStatusEnrichmentCurrent = current.fingerprint
	tracker.gitStatusEnrichmentJob = current
	tracker.gitStatusEnrichmentMu.Unlock()

	tracker.scheduleGitStatusEnrichment(replacement)

	tracker.gitStatusEnrichmentMu.Lock()
	queued := tracker.gitStatusEnrichmentNext
	tracker.gitStatusEnrichmentMu.Unlock()
	if queued != replacement {
		t.Fatalf("same-fingerprint correction queue = %p, want replacement %p", queued, replacement)
	}
	select {
	case <-replacement.done:
		t.Fatal("correction was marked complete before the failed attempt settled")
	default:
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
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	writeFile(t, repoDir, "broken.txt", "base broken\n")
	writeFile(t, repoDir, "healthy.txt", "base healthy\n")
	runGit(t, repoDir, "add", "broken.txt", "healthy.txt")
	runGit(t, repoDir, "commit", "-m", "Add changed files")
	runGit(t, repoDir, "update-ref", "refs/remotes/origin/main", "HEAD")
	writeFile(t, repoDir, "broken.txt", "base broken\nchanged broken\n")
	writeFile(t, repoDir, "healthy.txt", "base healthy\nchanged healthy\n")

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	failBrokenDiff := true
	tracker.gitStatusDiffOutput = func(ctx context.Context, workDir string, args ...string) (string, bool, error) {
		if failBrokenDiff && len(args) > 0 && args[0] == "diff" && args[len(args)-1] == "broken.txt" {
			return "", false, errors.New("injected transient diff failure")
		}
		return capDiffOutput(ctx, workDir, args...)
	}

	first, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if !errors.Is(err, errGitStatusDetailsUnavailable) {
		t.Fatalf("first details error = %v, want unavailable after one diff command fails", err)
	}
	if first.DetailState != gitStatusDetailUnavailable {
		t.Fatalf("first detail quality = %q, want unavailable", first.DetailState)
	}
	if first.Files["broken.txt"].DiffState != gitStatusDiffUnavailable {
		t.Fatalf("failed file state = %q, want unavailable; file = %+v; status = %+v", first.Files["broken.txt"].DiffState, first.Files["broken.txt"], first)
	}
	if first.Files["healthy.txt"].DiffState != gitStatusDiffReady || !strings.Contains(first.Files["healthy.txt"].Diff, "changed healthy") {
		t.Fatalf("healthy file details were lost after sibling command failure: %+v", first.Files["healthy.txt"])
	}

	failBrokenDiff = false
	retried, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if err != nil {
		t.Fatalf("retry on unchanged repository: %v", err)
	}
	if retried.DetailState != gitStatusDetailReady || retried.Files["broken.txt"].DiffState != gitStatusDiffReady ||
		!strings.Contains(retried.Files["broken.txt"].Diff, "changed broken") {
		t.Fatalf("retry did not repair failed details: %+v", retried)
	}
}

func TestWorkspaceTrackerOnlyRetriesUnavailableDetailsOnExplicitRefresh(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	writeFile(t, repoDir, "README.md", "base\n")
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "Add README")
	runGit(t, repoDir, "update-ref", "refs/remotes/origin/main", "HEAD")
	writeFile(t, repoDir, "README.md", "base\nchanged\n")

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	var failDiff atomic.Bool
	failDiff.Store(true)
	var diffCalls atomic.Int32
	tracker.gitStatusDiffOutput = func(ctx context.Context, workDir string, args ...string) (string, bool, error) {
		if len(args) > 0 && args[0] == "diff" && args[len(args)-1] == "README.md" {
			diffCalls.Add(1)
			if failDiff.Load() {
				return "", false, errors.New("injected transient diff failure")
			}
		}
		return capDiffOutput(ctx, workDir, args...)
	}

	first, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if !errors.Is(err, errGitStatusDetailsUnavailable) || first.DetailState != gitStatusDetailUnavailable {
		t.Fatalf("initial detail result = %+v, %v; want unavailable", first, err)
	}
	failedCalls := diffCalls.Load()
	if failedCalls == 0 {
		t.Fatal("initial enrichment did not attempt the changed file diff")
	}

	polled, err := tracker.GetGitStatus(context.Background(), true)
	if err != nil {
		t.Fatalf("ordinary fresh poll: %v", err)
	}
	if polled.DetailState != gitStatusDetailUnavailable {
		t.Fatalf("ordinary poll quality = %q, want unavailable until explicit retry", polled.DetailState)
	}
	if got := diffCalls.Load(); got != failedCalls {
		t.Fatalf("ordinary poll started more detail commands: got %d, want %d", got, failedCalls)
	}

	failDiff.Store(false)
	retried, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if err != nil {
		t.Fatalf("explicit unchanged-repository retry: %v", err)
	}
	if retried.DetailState != gitStatusDetailReady || retried.Files["README.md"].DiffState != gitStatusDiffReady || !strings.Contains(retried.Files["README.md"].Diff, "changed") {
		t.Fatalf("explicit retry did not repair details: %+v", retried)
	}
	if got := diffCalls.Load(); got <= failedCalls {
		t.Fatalf("explicit retry ran no diff command: calls = %d, previous = %d", got, failedCalls)
	}
}

func TestWorkspaceTrackerKeepsSupportedFilesWhenRawContentExceedsDiffBudget(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	largeContent := strings.Repeat("stable line content for a large tracked file\n", 72_000)
	writeFile(t, repoDir, "large.txt", largeContent)
	writeFile(t, repoDir, "small.txt", "small base\n")
	runGit(t, repoDir, "add", "large.txt", "small.txt")
	runGit(t, repoDir, "commit", "-m", "Add large and small files")
	runGit(t, repoDir, "update-ref", "refs/remotes/origin/main", "HEAD")
	writeFile(t, repoDir, "large.txt", largeContent+"small final edit\n")
	writeFile(t, repoDir, "small.txt", "small base\nsmall edit\n")

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	status, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if err != nil {
		t.Fatalf("enrich supported files: %v", err)
	}
	if status.DetailState != gitStatusDetailReady || status.BranchAdditions == 0 {
		t.Fatalf("detail quality/totals = %q / %d, want ready details and known positive totals", status.DetailState, status.BranchAdditions)
	}
	for _, path := range []string{"large.txt", "small.txt"} {
		file := status.Files[path]
		if file.DiffState != gitStatusDiffReady || !strings.Contains(file.Diff, "edit") {
			t.Errorf("%s details = %+v, want a ready small patch", path, file)
		}
	}
}

func TestWorkspaceTrackerKeepsDiffsReadyWithoutImplicitAheadBehindRef(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	runGit(t, repoDir, "branch", "--unset-upstream")
	runGit(t, repoDir, "update-ref", "-d", "refs/remotes/origin/main")
	writeFile(t, repoDir, "README.md", "# Test Repo\nchanged\n")

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	status, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if err != nil {
		t.Fatalf("enrich without implicit ahead/behind ref: %v", err)
	}
	if status.DetailState != gitStatusDetailReady || status.Files["README.md"].DiffState != gitStatusDiffReady {
		t.Fatalf("status = %+v, want ready file details despite absent implicit comparison ref", status)
	}
}

func TestWorkspaceTrackerQueuesRetryAfterUnavailablePublicationBeforeWorkerSettles(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	writeFile(t, repoDir, "README.md", "base\n")
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "Add README")
	runGit(t, repoDir, "update-ref", "refs/remotes/origin/main", "HEAD")
	writeFile(t, repoDir, "README.md", "base\nchanged\n")

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	var failDiff atomic.Bool
	var diffCalls atomic.Int32
	var unavailableHookCalls atomic.Int32
	failDiff.Store(true)
	tracker.gitStatusDiffOutput = func(ctx context.Context, workDir string, args ...string) (string, bool, error) {
		if failDiff.Load() && len(args) > 0 && args[0] == "diff" && args[len(args)-1] == "README.md" {
			diffCalls.Add(1)
			return "", false, errors.New("injected transient diff failure")
		}
		return capDiffOutput(ctx, workDir, args...)
	}
	firstUnavailable := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseFirst) })
	tracker.gitStatusAfterUnavailablePublication = func() {
		unavailableHookCalls.Add(1)
		select {
		case <-firstUnavailable:
			return
		default:
			close(firstUnavailable)
			<-releaseFirst
		}
	}
	joinedRetry := make(chan struct{})
	tracker.gitStatusDetailsWaitJoined = func() { close(joinedRetry) }

	initial, err := tracker.GetGitStatus(context.Background(), true)
	if err != nil || initial.DetailState != gitStatusDetailPending {
		t.Fatalf("initial basic status = %+v, %v; want pending", initial, err)
	}
	select {
	case <-firstUnavailable:
	case <-time.After(5 * time.Second):
		tracker.gitStatusEnrichmentMu.Lock()
		current := tracker.gitStatusEnrichmentJob
		currentFingerprint := tracker.gitStatusEnrichmentCurrent
		tracker.gitStatusEnrichmentMu.Unlock()
		t.Fatalf("unavailable publication hook not reached: hook_calls=%d diff_calls=%d status=%+v current=%+v fingerprint=%s", unavailableHookCalls.Load(), diffCalls.Load(), tracker.currentGitStatus(), current, currentFingerprint)
	}
	failDiff.Store(false)
	type detailResult struct {
		status types.GitStatusUpdate
		err    error
	}
	resultCh := make(chan detailResult, 1)
	go func() {
		status, err := tracker.GetGitStatusWithDetails(context.Background(), true)
		resultCh <- detailResult{status: status, err: err}
	}()
	waitForSignal(t, joinedRetry, "joined same-fingerprint retry")

	tracker.gitStatusEnrichmentMu.Lock()
	next := tracker.gitStatusEnrichmentNext
	current := tracker.gitStatusEnrichmentJob
	queued := next != nil && current != nil && next.fingerprint == current.fingerprint && next.explicitRetry
	tracker.gitStatusEnrichmentMu.Unlock()
	if !queued {
		t.Fatal("explicit refresh did not queue behind the still-settling unavailable job")
	}

	releaseOnce.Do(func() { close(releaseFirst) })
	select {
	case result := <-resultCh:
		if result.err != nil || result.status.DetailState != gitStatusDetailReady {
			t.Fatalf("same-fingerprint retry result = %+v, %v; want ready", result.status, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("same-fingerprint retry did not settle")
	}
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
