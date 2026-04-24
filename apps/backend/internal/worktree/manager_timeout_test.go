package worktree

import (
	"context"
	"os"
	"testing"
	"time"
)

// hangOnRevParseScript is the shared fake-git script body used by the timeout
// regression tests. It sleeps indefinitely on `git rev-parse` and no-ops on
// everything else (including `fetch`), so branchExists, currentBranch, and
// the pullBaseBranch path that calls them all exercise the hang scenario.
const hangOnRevParseScript = `
case "${1:-}" in
  rev-parse)
    sleep 30
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`

// TestBranchExists_RespectsContextDeadline verifies that branchExists cancels
// the underlying git subprocess when its caller-provided context expires.
func TestBranchExists_RespectsContextDeadline(t *testing.T) {
	scriptDir := writeFakeGitScript(t, hangOnRevParseScript)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	exists := mgr.branchExists(ctx, t.TempDir(), "main")
	elapsed := time.Since(start)

	if exists {
		t.Fatalf("branchExists() = true, want false on hanging git")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("branchExists() took %v, want <2s (ctx not propagated to subprocess)", elapsed)
	}
}

// TestCurrentBranch_RespectsContextDeadline verifies that currentBranch
// cancels the git subprocess when its caller-provided context expires.
func TestCurrentBranch_RespectsContextDeadline(t *testing.T) {
	scriptDir := writeFakeGitScript(t, hangOnRevParseScript)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	branch := mgr.currentBranch(ctx, t.TempDir())
	elapsed := time.Since(start)

	if branch != "" {
		t.Fatalf("currentBranch() = %q, want empty on hanging git", branch)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("currentBranch() took %v, want <2s (ctx not propagated to subprocess)", elapsed)
	}
}

// TestBranchExists_BoundedWhenCallerHasNoDeadline pins the behaviour of the
// Manager's internal inspectTimeout. With a background (never-cancelled)
// caller ctx, branchExists must still return within m.inspectTimeout so a
// future refactor that drops the wrapping timeout cannot silently regress
// the fix. We shrink inspectTimeout for the test to keep it fast.
func TestBranchExists_BoundedWhenCallerHasNoDeadline(t *testing.T) {
	scriptDir := writeFakeGitScript(t, hangOnRevParseScript)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	mgr.inspectTimeout = 300 * time.Millisecond

	start := time.Now()
	exists := mgr.branchExists(context.Background(), t.TempDir(), "main")
	elapsed := time.Since(start)

	if exists {
		t.Fatalf("branchExists() = true, want false on hanging git")
	}
	// Budget = inspectTimeout + WaitDelay for subprocess pipe cleanup + slack.
	if elapsed > 2*time.Second {
		t.Fatalf("branchExists() took %v, want <2s (inspectTimeout not applied)", elapsed)
	}
}

// TestCreate_HangingRevParseReleasesRepoLock is the core regression test for
// the reported symptom: a hung git rev-parse during Create must not keep the
// per-repo mutex locked indefinitely. Before the fix, branchExists and
// currentBranch ignored ctx, so a backend restart was the only way to clear
// the lock. After the fix, ctx cancellation propagates to the git subprocess
// and Create returns, releasing the lock for subsequent callers.
func TestCreate_HangingRevParseReleasesRepoLock(t *testing.T) {
	scriptDir := writeFakeGitScript(t, hangOnRevParseScript)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	mgr.fetchTimeout = 100 * time.Millisecond
	mgr.pullTimeout = 100 * time.Millisecond

	repoPath := t.TempDir()
	if err := os.MkdirAll(repoPath+"/.git", 0755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	req := CreateRequest{
		TaskID:             "task-a",
		SessionID:          "sess-a",
		TaskTitle:          "hang repro",
		RepositoryPath:     repoPath,
		BaseBranch:         "main",
		PullBeforeWorktree: true,
	}

	// Short caller deadline exercises ctx propagation through branchExists
	// and currentBranch. Before the fix these ignored ctx and slept the full
	// 30s in the fake git script, holding repoLock the entire time.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = mgr.Create(ctx, req)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("Create() err = nil, want error on hanging git")
	}
	// Budget: inspect timeout + WaitDelay per subprocess kill, bounded by a
	// few subprocess kills and any lock bookkeeping. Pre-fix this was ~30s.
	if elapsed > 10*time.Second {
		t.Fatalf("Create() took %v, want <10s (lock held during hung git)", elapsed)
	}

	// Confirm the repo lock was released: a second Create call must be able
	// to acquire it without waiting. We use a fresh context so the call can
	// run briefly, then cancel; all we care about is that Lock() didn't
	// block us out.
	lockCtx, lockCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer lockCancel()

	lockStart := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = mgr.Create(lockCtx, req)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("second Create() stuck on repo lock (elapsed %v)", time.Since(lockStart))
	}
}
