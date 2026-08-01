package process

import (
	"context"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestMonitorLoop_OverlapGuard_SkipsTick verifies that the CAS overlap guard
// in monitorLoop prevents process pile-up by rejecting ticks when the flag is set.
func TestMonitorLoop_OverlapGuard_SkipsTick(t *testing.T) {
	isolateTestGitEnv(t)

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)

	// Simulate a long-running cycle: flag is 1
	atomic.StoreInt32(&wt.monitorRunning, 1)

	// CAS should fail — tick would be skipped in the loop
	if atomic.CompareAndSwapInt32(&wt.monitorRunning, 0, 1) {
		t.Error("CAS should have failed when monitorRunning is 1 (tick should be skipped)")
	}

	// Flag should still be 1
	if atomic.LoadInt32(&wt.monitorRunning) != 1 {
		t.Error("expected monitorRunning to remain 1")
	}

	// After the simulated cycle completes, flag is reset
	atomic.StoreInt32(&wt.monitorRunning, 0)

	// Now CAS should succeed — tick would proceed
	if !atomic.CompareAndSwapInt32(&wt.monitorRunning, 0, 1) {
		t.Error("CAS should have succeeded when monitorRunning is 0")
	}

	// Verify monitorTick resets the flag via defer
	atomic.StoreInt32(&wt.monitorRunning, 1)
	ctx := context.Background()
	var consecutiveFailures int
	lastState, _ := wt.getWorkspaceState(ctx)
	wt.monitorTick(ctx, &lastState, &consecutiveFailures)
	if atomic.LoadInt32(&wt.monitorRunning) != 0 {
		t.Error("monitorTick should have reset monitorRunning to 0 via defer")
	}
}

// TestPollGitChanges_OverlapGuard_SkipsTick verifies that the CAS overlap guard
// in pollGitChanges prevents process pile-up by rejecting ticks when the flag is set.
func TestPollGitChanges_OverlapGuard_SkipsTick(t *testing.T) {
	isolateTestGitEnv(t)

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)

	// Simulate a long-running cycle: flag is 1
	atomic.StoreInt32(&wt.gitPollRunning, 1)

	// CAS should fail — tick would be skipped
	if atomic.CompareAndSwapInt32(&wt.gitPollRunning, 0, 1) {
		t.Error("CAS should have failed when gitPollRunning is 1 (tick should be skipped)")
	}

	// After the simulated cycle completes, flag is reset
	atomic.StoreInt32(&wt.gitPollRunning, 0)

	// Now CAS should succeed
	if !atomic.CompareAndSwapInt32(&wt.gitPollRunning, 0, 1) {
		t.Error("CAS should have succeeded when gitPollRunning is 0")
	}

	// Verify gitPollTick resets the flag via defer
	atomic.StoreInt32(&wt.gitPollRunning, 1)
	ctx := context.Background()
	var consecutiveFailures int
	wt.gitPollTick(ctx, &consecutiveFailures)
	if atomic.LoadInt32(&wt.gitPollRunning) != 0 {
		t.Error("gitPollTick should have reset gitPollRunning to 0 via defer")
	}
}

// TestTryUpdateGitStatus_SkipsWhenLocked verifies that tryUpdateGitStatus returns
// immediately when another update is already in progress, preventing both polling
// loops from running expensive git commands simultaneously.
func TestTryUpdateGitStatus_SkipsWhenLocked(t *testing.T) {
	isolateTestGitEnv(t)

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	// Acquire the updateMu to simulate an in-progress update
	wt.updateMu.Lock()

	// tryUpdateGitStatus should return immediately without blocking
	done := make(chan struct{})
	go func() {
		wt.tryUpdateGitStatus(ctx)
		close(done)
	}()

	select {
	case <-done:
		// tryUpdateGitStatus returned immediately — correct behavior
	case <-time.After(1 * time.Second):
		t.Fatal("tryUpdateGitStatus blocked when updateMu was held; expected it to skip")
	}

	wt.updateMu.Unlock()
}

// TestRefreshGitStatus_BlocksUntilLockAvailable verifies that RefreshGitStatus
// (used by explicit user operations) waits for the lock rather than skipping.
func TestRefreshGitStatus_BlocksUntilLockAvailable(t *testing.T) {
	isolateTestGitEnv(t)

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	// Acquire the updateMu to simulate an in-progress update
	wt.updateMu.Lock()

	refreshDone := make(chan struct{})
	started := make(chan struct{})
	go func() {
		close(started) // signal: goroutine is scheduled
		wt.RefreshGitStatus(ctx)
		close(refreshDone)
	}()

	// Ensure goroutine is running and blocked on Lock()
	<-started

	// Verify RefreshGitStatus hasn't completed (it should be blocked on the mutex)
	select {
	case <-refreshDone:
		t.Fatal("RefreshGitStatus completed while lock was held; expected it to block")
	default:
		// Good — still blocked
	}

	// Release the lock — RefreshGitStatus should now complete
	wt.updateMu.Unlock()

	select {
	case <-refreshDone:
		// Success — completed after lock was released
	case <-time.After(5 * time.Second):
		t.Fatal("RefreshGitStatus did not complete after lock was released")
	}
}

// TestUpdateMu_PreventsConcurrentUpdates verifies that two concurrent
// tryUpdateGitStatus calls don't both execute — only one should proceed.
func TestUpdateMu_PreventsConcurrentUpdates(t *testing.T) {
	isolateTestGitEnv(t)

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	// Run initial update so status is cached (avoid lazy-init paths)
	wt.updateGitStatus(ctx)

	// Track how many concurrent updates are active
	var concurrentCount int32
	var maxConcurrent int32
	var totalUpdates int32

	const goroutines = 10
	var wg sync.WaitGroup

	// Use a barrier to release all goroutines simultaneously
	barrier := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-barrier
			if wt.updateMu.TryLock() {
				current := atomic.AddInt32(&concurrentCount, 1)
				atomic.AddInt32(&totalUpdates, 1)
				// Track peak concurrency (CAS loop)
				for old := atomic.LoadInt32(&maxConcurrent); current > old && !atomic.CompareAndSwapInt32(&maxConcurrent, old, current); old = atomic.LoadInt32(&maxConcurrent) {
				}
				// Simulate work — brief sleep ensures goroutines overlap
				time.Sleep(10 * time.Millisecond)
				atomic.AddInt32(&concurrentCount, -1)
				wt.updateMu.Unlock()
			}
		}()
	}

	// Release all goroutines simultaneously
	close(barrier)
	wg.Wait()

	if atomic.LoadInt32(&maxConcurrent) > 1 {
		t.Errorf("expected max concurrency of 1, got %d", atomic.LoadInt32(&maxConcurrent))
	}

	// Not all goroutines should have acquired the lock (some should have been skipped)
	total := atomic.LoadInt32(&totalUpdates)
	if total == int32(goroutines) {
		t.Errorf("expected some goroutines to be skipped, but all %d acquired the lock", goroutines)
	}
	t.Logf("TryLock acquired by %d/%d goroutines (others correctly skipped)", total, goroutines)
}

// snapshotIndexHash reads a poll snapshot and returns its index hash, failing
// the test if the underlying git read errored.
func snapshotIndexHash(t *testing.T, wt *WorkspaceTracker, ctx context.Context) string {
	t.Helper()
	snap, err := wt.readGitPollSnapshot(ctx)
	if err != nil {
		t.Fatalf("readGitPollSnapshot failed: %v", err)
	}
	return snap.indexHash
}

// TestGitPollSnapshot_ExcludesUntrackedFiles verifies that the poll snapshot uses
// --untracked-files=no, so untracked files don't appear in the hash. This prevents
// the expensive directory traversal that --untracked-files=all performs.
func TestGitPollSnapshot_ExcludesUntrackedFiles(t *testing.T) {
	isolateTestGitEnv(t)

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	// Get baseline hash with no changes
	baseHash := snapshotIndexHash(t, wt, ctx)
	if baseHash == "" {
		t.Fatal("expected non-empty hash for clean repo")
	}

	// Create an untracked file — hash should NOT change (--untracked-files=no)
	writeFile(t, repoDir, "untracked.txt", "untracked content")
	untrackedHash := snapshotIndexHash(t, wt, ctx)
	if untrackedHash != baseHash {
		t.Errorf("hash changed after adding untracked file; expected --untracked-files=no to exclude it\n  before: %s\n  after:  %s", baseHash, untrackedHash)
	}

	// Stage the file — hash SHOULD change (staged files show as A, not ??)
	runGit(t, repoDir, "add", "untracked.txt")
	stagedHash := snapshotIndexHash(t, wt, ctx)
	if stagedHash == baseHash {
		t.Error("hash did not change after staging a file; expected staging to be detected")
	}
}

// TestGitPollSnapshot_DetectsModifiedFiles verifies that the poll snapshot still
// detects tracked file modifications even with --untracked-files=no.
func TestGitPollSnapshot_DetectsModifiedFiles(t *testing.T) {
	isolateTestGitEnv(t)

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	baseHash := snapshotIndexHash(t, wt, ctx)

	// Modify a tracked file — hash should change
	writeFile(t, repoDir, "README.md", "# Modified")
	modifiedHash := snapshotIndexHash(t, wt, ctx)
	if modifiedHash == baseHash {
		t.Error("hash did not change after modifying tracked file")
	}
}

// TestGitPollSnapshot_ReadsHeadAndBranch verifies the two values that used to
// come from separate rev-parse / symbolic-ref spawns are now parsed out of the
// porcelain v2 branch headers, including the detached-HEAD sentinel.
func TestGitPollSnapshot_ReadsHeadAndBranch(t *testing.T) {
	isolateTestGitEnv(t)

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	snap, err := wt.readGitPollSnapshot(ctx)
	if err != nil {
		t.Fatalf("readGitPollSnapshot failed: %v", err)
	}
	if len(snap.headSHA) != 40 {
		t.Errorf("expected a 40-char HEAD SHA, got %q", snap.headSHA)
	}
	if snap.branch == "" {
		t.Error("expected a branch name on a freshly initialized repo")
	}

	// Detaching HEAD must clear the branch, not report "(detached)".
	runGit(t, repoDir, "checkout", "--detach")
	detached, err := wt.readGitPollSnapshot(ctx)
	if err != nil {
		t.Fatalf("readGitPollSnapshot failed after detach: %v", err)
	}
	if detached.branch != "" {
		t.Errorf("expected empty branch in detached HEAD state, got %q", detached.branch)
	}
	if detached.headSHA != snap.headSHA {
		t.Errorf("HEAD changed across detach: %q -> %q", snap.headSHA, detached.headSHA)
	}
}

// TestParseGitPollSnapshot_Sentinels covers the header parsing without needing a
// repository in each of the states git can report.
func TestParseGitPollSnapshot_Sentinels(t *testing.T) {
	unborn := parseGitPollSnapshot([]byte("# branch.oid (initial)\n# branch.head main\n"))
	if unborn.headSHA != "" {
		t.Errorf("expected empty HEAD for an unborn branch, got %q", unborn.headSHA)
	}
	if unborn.branch != "main" {
		t.Errorf("expected branch %q, got %q", "main", unborn.branch)
	}

	// branch.ab moves on fetch without the index changing; it must not leak
	// into the hash.
	withAB := parseGitPollSnapshot([]byte("# branch.oid abc\n# branch.head main\n# branch.ab +0 -0\n"))
	withoutAB := parseGitPollSnapshot([]byte("# branch.oid abc\n# branch.head main\n# branch.ab +3 -1\n"))
	if withAB.indexHash != withoutAB.indexHash {
		t.Error("index hash changed with branch.ab; headers must be excluded from the hash")
	}

	// --no-ahead-behind makes git print "+? -?" instead of real counts. It must
	// parse like any other header and leave the hash alone, so that dropping the
	// flag (and paying for the revision walk again) stays a deliberate act.
	unknownAB := parseGitPollSnapshot([]byte("# branch.oid abc\n# branch.head main\n# branch.ab +? -?\n"))
	if unknownAB.indexHash != withAB.indexHash {
		t.Error("index hash changed with an unknown branch.ab; the header must be ignored")
	}
	if unknownAB.headSHA != "abc" || unknownAB.branch != "main" {
		t.Errorf("unknown branch.ab disturbed the other headers: head=%q branch=%q", unknownAB.headSHA, unknownAB.branch)
	}

	// Entry lines must drive the hash.
	entry := parseGitPollSnapshot([]byte("# branch.oid abc\n# branch.head main\n1 .M N... 100644 100644 100644 aaa bbb file.txt\n"))
	if entry.indexHash == withAB.indexHash {
		t.Error("index hash did not change when an entry line was present")
	}
}

// TestOverlapGuard_ResetsAfterNormalCycle verifies that the atomic overlap guard
// is properly reset after a normal poll cycle completes, allowing the next tick to run.
func TestOverlapGuard_ResetsAfterNormalCycle(t *testing.T) {
	isolateTestGitEnv(t)

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	wt.filePollInterval = 50 * time.Millisecond
	wt.gitPollInterval = 50 * time.Millisecond
	// Default mode is slow (30s) — set fast so the loops actually tick at 50ms.
	wt.SetPollMode(PollModeFast)

	wt.Start(context.Background())

	// Let several cycles run normally
	time.Sleep(300 * time.Millisecond)

	// Stop the tracker (this cancels wt.cancelCtx used by the loops)
	wt.Stop()

	// After Stop(), both flags should be 0 (cycles completed before shutdown)
	if atomic.LoadInt32(&wt.monitorRunning) != 0 {
		t.Error("monitorRunning flag stuck at 1 after normal cycles")
	}
	if atomic.LoadInt32(&wt.gitPollRunning) != 0 {
		t.Error("gitPollRunning flag stuck at 1 after normal cycles")
	}
}

// TestOverlapGuard_WorkDirDeletedResetsFlag verifies that the overlap guard
// is reset when the work directory is deleted, so the goroutine exits cleanly.
func TestOverlapGuard_WorkDirDeletedResetsFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows refuses to unlink a directory while a process holds a handle inside it; the scenario this test exercises cannot occur on Windows")
	}
	isolateTestGitEnv(t)

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	wt.filePollInterval = 50 * time.Millisecond
	wt.gitPollInterval = 50 * time.Millisecond
	// Default mode is slow (30s) — set fast so the loops actually tick at 50ms.
	wt.SetPollMode(PollModeFast)

	wt.Start(context.Background())
	defer wt.Stop()

	// Let a cycle or two complete
	time.Sleep(150 * time.Millisecond)

	// Delete the work directory
	if err := os.RemoveAll(repoDir); err != nil {
		t.Fatalf("failed to remove workdir: %v", err)
	}

	// Both goroutines should exit via workDirExists() check
	done := make(chan struct{})
	go func() {
		wt.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Clean exit
	case <-time.After(5 * time.Second):
		t.Fatal("goroutines did not exit after workdir deletion")
	}

	// Flags should be reset to 0
	if atomic.LoadInt32(&wt.monitorRunning) != 0 {
		t.Error("monitorRunning flag not reset after workdir deletion exit")
	}
	if atomic.LoadInt32(&wt.gitPollRunning) != 0 {
		t.Error("gitPollRunning flag not reset after workdir deletion exit")
	}
}
