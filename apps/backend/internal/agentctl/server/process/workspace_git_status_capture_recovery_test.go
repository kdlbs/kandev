package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/subproc"
	"go.uber.org/zap/zapcore"
)

type gitCaptureResult struct {
	status types.GitStatusUpdate
	err    error
}

const gitCaptureTestWaitTimeout = 30 * time.Second

// @covers AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.1, AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.4
func TestWorkspaceTrackerGitStatusCaptureRecoveryRetriesOneEvidenceChange(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repoDir, "capture-retry.txt", "captured before the index change\n")
	log, observed := newObservedTestLogger(t)
	tracker := NewWorkspaceTracker(repoDir, log)
	t.Cleanup(tracker.Stop)
	var captureCount atomic.Int32
	var classes []subproc.GitWorkClass
	mutationErrors := make(chan error, 2)
	tracker.gitStatusBetweenQueries = func(ctx context.Context) {
		classes = append(classes, gitWorkClass(ctx))
		if captureCount.Add(1) == 1 {
			mutationErrors <- runGitCaptureMutation(repoDir, "add", "capture-retry.txt")
			return
		}
		mutationErrors <- nil
	}

	status, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if err != nil {
		t.Fatalf("GetGitStatusWithDetails after one index change: %v", err)
	}
	if got := captureCount.Load(); got != 2 {
		t.Fatalf("basic captures = %d, want one failed capture and one correction", got)
	}
	assertGitCaptureMutationErrors(t, mutationErrors, int(captureCount.Load()))
	if len(classes) != 2 || classes[0] != subproc.GitInteractive || classes[1] != classes[0] {
		t.Fatalf("capture admission classes = %v, want the same interactive class", classes)
	}
	if status.StatusState != gitStatusStateReady || !status.FilesComplete || status.DetailState != gitStatusDetailReady {
		t.Fatalf("recovered status quality = %q / %v / %q, want ready / complete / ready", status.StatusState, status.FilesComplete, status.DetailState)
	}
	file, ok := status.Files["capture-retry.txt"]
	if !ok || !file.Staged || file.Status != "added" || !strings.Contains(file.Diff, "captured before the index change") {
		t.Fatalf("recovered staged file = %#v, want the complete staged capture", file)
	}
	if got := observed.FilterMessage("workspace Git status evidence changed; retrying basic capture").FilterLevelExact(zapcore.DebugLevel).Len(); got != 1 {
		t.Fatalf("corrective capture debug entries = %d, want one", got)
	}
	assertNoIndexSnapshots(t, filepath.Dir(tracker.gitIndexPath))
}

// @covers AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.2
func TestWorkspaceTrackerGitStatusCaptureRecoveryBoundsContinuousEvidenceChanges(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repoDir, "capture-churn.txt", "initial\n")
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	var captureCount atomic.Int32
	mutationErrors := make(chan error, 2)
	tracker.gitStatusBetweenQueries = func(context.Context) {
		attempt := captureCount.Add(1)
		err := writeGitCaptureMutationFile(repoDir, "capture-churn.txt", fmt.Sprintf("mutation-%d\n", attempt))
		if err == nil {
			err = runGitCaptureMutation(repoDir, "add", "capture-churn.txt")
		}
		mutationErrors <- err
	}

	status, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if !errors.Is(err, errGitStatusEvidenceChanged) {
		t.Fatalf("GetGitStatusWithDetails error = %v, want exhausted evidence-change error", err)
	}
	if got := captureCount.Load(); got != 2 {
		t.Fatalf("basic captures = %d, want exactly two", got)
	}
	assertGitCaptureMutationErrors(t, mutationErrors, int(captureCount.Load()))
	if status.StatusState == gitStatusStateReady || tracker.currentGitStatus().StatusState == gitStatusStateReady {
		t.Fatalf("unstable capture became ready: returned=%+v current=%+v", status, tracker.currentGitStatus())
	}
	assertNoIndexSnapshots(t, filepath.Dir(tracker.gitIndexPath))
}

// @covers AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.2, AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.4
func TestWorkspaceTrackerGitStatusCaptureRecoveryUsesReplacementIdentity(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	initialHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	var captureCount atomic.Int32
	mutationErrors := make(chan error, 2)
	tracker.gitStatusBetweenQueries = func(context.Context) {
		if captureCount.Add(1) == 1 {
			mutationErrors <- runGitCaptureMutation(repoDir, "checkout", "-b", "capture-replacement")
			return
		}
		mutationErrors <- nil
	}

	status, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if err != nil {
		t.Fatalf("GetGitStatusWithDetails after branch replacement: %v", err)
	}
	if got := captureCount.Load(); got != 2 {
		t.Fatalf("basic captures = %d, want one failed capture and one correction", got)
	}
	assertGitCaptureMutationErrors(t, mutationErrors, int(captureCount.Load()))
	if status.Branch != "capture-replacement" || status.HeadCommit != initialHead {
		t.Fatalf("recovered identity = branch %q, head %q; want replacement branch at %q", status.Branch, status.HeadCommit, initialHead)
	}
	assertNoIndexSnapshots(t, filepath.Dir(tracker.gitIndexPath))
}

// @covers AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.3
func TestWorkspaceTrackerGitStatusCaptureRecoverySharesWaiters(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repoDir, "shared-retry.txt", "shared observation\n")
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	entered := make(chan struct{})
	release := make(chan struct{})
	secondJoined := make(chan struct{})
	var captureCount atomic.Int32
	var waiterCount atomic.Int32
	var releaseOnce sync.Once
	mutationErrors := make(chan error, 2)
	tracker.gitStatusBetweenQueries = func(context.Context) {
		if captureCount.Add(1) == 1 {
			close(entered)
			<-release
			mutationErrors <- runGitCaptureMutation(repoDir, "add", "shared-retry.txt")
			return
		}
		mutationErrors <- nil
	}
	tracker.gitStatusWaiterJoined = func() {
		if waiterCount.Add(1) == 2 {
			close(secondJoined)
		}
	}
	defer releaseOnce.Do(func() { close(release) })

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	firstResult := make(chan gitCaptureResult, 1)
	go func() {
		status, err := tracker.GetGitStatusWithDetails(firstCtx, true)
		firstResult <- gitCaptureResult{status: status, err: err}
	}()
	waitForGitCaptureSignal(t, entered, "first capture did not reach its mutation barrier")
	secondResult := make(chan gitCaptureResult, 1)
	go func() {
		status, err := tracker.GetGitStatusWithDetails(context.Background(), true)
		secondResult <- gitCaptureResult{status: status, err: err}
	}()
	waitForGitCaptureSignal(t, secondJoined, "second caller did not join the shared observation")
	cancelFirst()
	if first := waitForGitCaptureResult(t, firstResult); !errors.Is(first.err, context.Canceled) {
		t.Fatalf("canceled waiter error = %v, want context canceled", first.err)
	}
	releaseOnce.Do(func() { close(release) })
	second := waitForGitCaptureResult(t, secondResult)
	if second.err != nil {
		t.Fatalf("remaining waiter error = %v", second.err)
	}
	if second.status.DetailState != gitStatusDetailReady || !second.status.Files["shared-retry.txt"].Staged {
		t.Fatalf("remaining waiter status = %+v, want the recovered enriched result", second.status)
	}
	if got := captureCount.Load(); got != 2 {
		t.Fatalf("basic captures = %d, want one shared correction", got)
	}
	assertGitCaptureMutationErrors(t, mutationErrors, int(captureCount.Load()))
	assertNoIndexSnapshots(t, filepath.Dir(tracker.gitIndexPath))
}

// @covers AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.4
func TestWorkspaceTrackerGitStatusCaptureRecoveryKeepsSharedDeadline(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repoDir, "deadline-retry.txt", "deadline observation\n")
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	tracker.gitStatusObserveTimeout = 2 * time.Second
	var captureCount atomic.Int32
	var deadlines [2]time.Time
	mutationErrors := make(chan error, 2)
	tracker.gitStatusBetweenQueries = func(ctx context.Context) {
		attempt := captureCount.Add(1)
		if deadline, ok := ctx.Deadline(); ok {
			deadlines[attempt-1] = deadline
		}
		if attempt == 1 {
			mutationErrors <- runGitCaptureMutation(repoDir, "add", "deadline-retry.txt")
			return
		}
		<-ctx.Done()
		mutationErrors <- nil
	}

	status, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("GetGitStatusWithDetails error = %v, want the shared observation deadline", err)
	}
	if got := captureCount.Load(); got != 2 {
		t.Fatalf("basic captures = %d, want the initial and one corrective capture", got)
	}
	assertGitCaptureMutationErrors(t, mutationErrors, int(captureCount.Load()))
	if deadlines[0].IsZero() || !deadlines[0].Equal(deadlines[1]) {
		t.Fatalf("capture deadlines = %v and %v, want one shared deadline", deadlines[0], deadlines[1])
	}
	if status.StatusState == gitStatusStateReady || tracker.currentGitStatus().StatusState == gitStatusStateReady {
		t.Fatalf("expired capture became ready: returned=%+v current=%+v", status, tracker.currentGitStatus())
	}
	assertNoIndexSnapshots(t, filepath.Dir(tracker.gitIndexPath))
}

// @covers AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.4
func TestWorkspaceTrackerGitStatusCaptureRecoveryStopsOnShutdown(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repoDir, "shutdown-retry.txt", "shutdown observation\n")
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	entered := make(chan struct{})
	release := make(chan struct{})
	var captureCount atomic.Int32
	var releaseOnce sync.Once
	mutationErrors := make(chan error, 1)
	tracker.gitStatusBetweenQueries = func(context.Context) {
		captureCount.Add(1)
		close(entered)
		<-release
		mutationErrors <- runGitCaptureMutation(repoDir, "add", "shutdown-retry.txt")
	}
	defer releaseOnce.Do(func() { close(release) })
	resultCh := make(chan error, 1)
	go func() {
		_, err := tracker.GetGitStatusWithDetails(context.Background(), true)
		resultCh <- err
	}()
	waitForGitCaptureSignal(t, entered, "capture did not reach its shutdown barrier")
	stopDone := make(chan struct{})
	go func() {
		tracker.Stop()
		close(stopDone)
	}()
	waitForGitCaptureSignal(t, tracker.cancelCtx.Done(), "tracker shutdown did not cancel the observation")
	releaseOnce.Do(func() { close(release) })
	if err := waitForGitCaptureError(t, resultCh); !errors.Is(err, context.Canceled) {
		t.Fatalf("capture error after tracker shutdown = %v, want context canceled", err)
	}
	waitForGitCaptureSignal(t, stopDone, "tracker did not finish shutdown")
	if got := captureCount.Load(); got != 1 {
		t.Fatalf("basic capture barriers = %d, want no corrective capture after shutdown", got)
	}
	assertGitCaptureMutationErrors(t, mutationErrors, int(captureCount.Load()))
	if tracker.currentGitStatus().StatusState == gitStatusStateReady {
		t.Fatal("shutdown capture published a ready status")
	}
	assertNoIndexSnapshots(t, filepath.Dir(tracker.gitIndexPath))
}

func runGitCaptureMutation(repoDir string, args ...string) error {
	commandArgs := append([]string{"-C", repoDir}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func writeGitCaptureMutationFile(repoDir, path, content string) error {
	return os.WriteFile(filepath.Join(repoDir, path), []byte(content), 0o600)
}

func assertGitCaptureMutationErrors(t *testing.T, results <-chan error, count int) {
	t.Helper()
	for range count {
		if err := <-results; err != nil {
			t.Fatalf("test mutation failed: %v", err)
		}
	}
}

// @covers AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.4
func TestWorkspaceTrackerGitStatusCaptureRecoveryDoesNotRetryPermissionFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Git command wrapper uses a POSIX shell")
	}
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repoDir, "permission-check.txt", "content\n")
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	callCount := installFailOnceGitStatus(t)

	_, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if err == nil || errors.Is(err, errGitStatusEvidenceChanged) {
		t.Fatalf("GetGitStatusWithDetails error = %v, want the injected permission failure", err)
	}
	if got := callCount(); got != 1 {
		t.Fatalf("tracked status command attempts = %d, want one after a non-evidence failure", got)
	}
}

// @covers AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.4
func TestWorkspaceTrackerGitStatusCaptureRecoveryDoesNotRetryMissingRepository(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Git command wrapper uses a POSIX shell")
	}
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	commandCount := installCountingGitCommands(t)
	if err := os.Rename(filepath.Join(repoDir, ".git"), filepath.Join(repoDir, ".git-unavailable")); err != nil {
		t.Fatalf("remove Git repository context: %v", err)
	}

	_, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if err == nil || errors.Is(err, errGitStatusEvidenceChanged) {
		t.Fatalf("GetGitStatusWithDetails error = %v, want a missing-repository failure", err)
	}
	if got := commandCount(); got != 1 {
		t.Fatalf("Git commands after repository removal = %d, want one", got)
	}
	if tracker.currentGitStatus().StatusState == gitStatusStateReady {
		t.Fatal("missing repository was published as ready")
	}
}

func installFailOnceGitStatus(t *testing.T) func() int {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("POSIX shell unavailable: %v", err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find Git executable: %v", err)
	}
	binDir := t.TempDir()
	marker := filepath.Join(binDir, "failed-once")
	calls := filepath.Join(binDir, "status-calls")
	shim := filepath.Join(binDir, "git")
	script := fmt.Sprintf("#!/bin/sh\ncase \" $* \" in\n  *\" status --porcelain --untracked-files=no \"*)\n    printf x >> %q\n    if [ ! -e %q ]; then : > %q; printf 'Permission denied\\n' >&2; exit 128; fi\n    ;;\nesac\nexec %q \"$@\"\n", calls, marker, marker, realGit)
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatalf("write Git command wrapper: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() int {
		data, err := os.ReadFile(calls)
		if err != nil {
			return 0
		}
		return len(data)
	}
}

func installCountingGitCommands(t *testing.T) func() int {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find Git executable: %v", err)
	}
	binDir := t.TempDir()
	calls := filepath.Join(binDir, "git-calls")
	shim := filepath.Join(binDir, "git")
	script := fmt.Sprintf("#!/bin/sh\nprintf x >> %q\nexec %q \"$@\"\n", calls, realGit)
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatalf("write Git command wrapper: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() int {
		data, err := os.ReadFile(calls)
		if err != nil {
			return 0
		}
		return len(data)
	}
}

func waitForGitCaptureSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(gitCaptureTestWaitTimeout):
		t.Fatal(message)
	}
}

func waitForGitCaptureResult(t *testing.T, resultCh <-chan gitCaptureResult) gitCaptureResult {
	t.Helper()
	select {
	case result := <-resultCh:
		return result
	case <-time.After(gitCaptureTestWaitTimeout):
		t.Fatal("Git status observation did not complete")
		return gitCaptureResult{}
	}
}

func waitForGitCaptureError(t *testing.T, resultCh <-chan error) error {
	t.Helper()
	select {
	case err := <-resultCh:
		return err
	case <-time.After(gitCaptureTestWaitTimeout):
		t.Fatal("Git status observation did not complete")
		return nil
	}
}
