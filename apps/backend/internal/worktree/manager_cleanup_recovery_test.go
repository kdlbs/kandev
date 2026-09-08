package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type cleanupOrderRecordingScriptHandler struct {
	cleanupRuns int
}

func (h *cleanupOrderRecordingScriptHandler) ExecuteSetupScript(context.Context, ScriptExecutionRequest) error {
	return nil
}

func (h *cleanupOrderRecordingScriptHandler) ExecuteCleanupScript(context.Context, ScriptExecutionRequest) error {
	h.cleanupRuns++
	return nil
}

type cancellingCleanupScriptHandler struct {
	cancel context.CancelFunc
}

type failingReleaseStore struct {
	*SQLiteStore
	failUpdate bool
}

func (s *failingReleaseStore) UpdateWorktree(ctx context.Context, wt *Worktree) error {
	if s.failUpdate {
		return errors.New("injected release failure")
	}
	return s.SQLiteStore.UpdateWorktree(ctx, wt)
}

type swappingCleanupScriptHandler struct{}

func (h *swappingCleanupScriptHandler) ExecuteSetupScript(context.Context, ScriptExecutionRequest) error {
	return nil
}

func (h *swappingCleanupScriptHandler) ExecuteCleanupScript(_ context.Context, req ScriptExecutionRequest) error {
	replacement := req.WorkingDir + "-replacement"
	if err := os.Rename(req.WorkingDir, replacement); err != nil {
		return err
	}
	if err := os.MkdirAll(req.WorkingDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(req.WorkingDir, "replacement.txt"), []byte("preserve me\n"), 0o644)
}

func (h *cancellingCleanupScriptHandler) ExecuteSetupScript(context.Context, ScriptExecutionRequest) error {
	return nil
}

func (h *cancellingCleanupScriptHandler) ExecuteCleanupScript(context.Context, ScriptExecutionRequest) error {
	h.cancel()
	return nil
}

func TestCleanupWorktrees_RecoversAfterPathOnlyRemoval(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-partial", "session-partial", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-partial", "session-partial")
	wt.CleanupHeadOID = strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))

	if err := os.RemoveAll(wt.Path); err != nil {
		t.Fatalf("remove worktree path without shared Git metadata: %v", err)
	}
	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("recover partial cleanup: %v", err)
	}

	assertNoCleanupRegistration(t, wt.RepositoryPath, wt.Path)
	assertCleanupBranchAbsent(t, wt.RepositoryPath, wt.Branch)
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
}

func TestCleanupWorktrees_RejectsPathlessRetryWithoutImmutableHead(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-no-snapshot", "session-no-snapshot", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-no-snapshot", "session-no-snapshot")

	if err := os.RemoveAll(wt.Path); err != nil {
		t.Fatalf("remove worktree path without shared Git metadata: %v", err)
	}
	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err == nil {
		t.Fatal("cleanup accepted pathless worktree without immutable cleanup identity")
	}
	assertCleanupBranchPresent(t, wt.RepositoryPath, wt.Branch)
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
}

func TestCleanupWorktreesPreservingBranches_RetriesAfterReleaseFailure(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-preserve-retry", "session-preserve-retry", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-preserve-retry", "session-preserve-retry")

	mgr.store = &failingReleaseStore{SQLiteStore: store, failUpdate: true}
	if err := mgr.CleanupWorktreesPreservingBranches(context.Background(), []*Worktree{wt}); err == nil {
		t.Fatal("cleanup succeeded despite injected reference-release failure")
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree path remains after first removal: %v", err)
	}
	assertCleanupBranchPresent(t, wt.RepositoryPath, wt.Branch)

	mgr.store = store
	if err := mgr.CleanupWorktreesPreservingBranches(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("retry pathless branch-preserving cleanup: %v", err)
	}
	assertCleanupBranchPresent(t, wt.RepositoryPath, wt.Branch)
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
}

func TestCleanupWorktrees_PreservesReplacementAfterPostAuditPathSwap(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-path-swap", "session-path-swap", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-path-swap", "session-path-swap")
	wt.CleanupHeadOID = strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))

	mgr.SetRepositoryProvider(&fakeRepoProvider{repo: &Repository{ID: wt.RepositoryID, CleanupScript: "swap"}})
	mgr.SetScriptMessageHandler(&swappingCleanupScriptHandler{})
	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err == nil {
		t.Fatal("cleanup accepted a path replacement after audit")
	}

	replacement := filepath.Join(filepath.Dir(wt.Path), filepath.Base(wt.Path)+"-replacement")
	if got, err := os.ReadFile(filepath.Join(wt.Path, "replacement.txt")); err != nil || string(got) != "preserve me\n" {
		t.Fatalf("replacement path changed: contents=%q err=%v", got, err)
	}
	if _, err := os.Stat(replacement); err != nil {
		t.Fatalf("renamed original worktree missing: %v", err)
	}
	assertCleanupBranchPresent(t, wt.RepositoryPath, wt.Branch)
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
}

func TestCleanupWorktrees_PartialFailureRemainsRetryable(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-retry", "session-retry", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-retry", "session-retry")
	wt.CleanupHeadOID = strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))

	if err := os.RemoveAll(wt.Path); err != nil {
		t.Fatalf("remove worktree path without shared Git metadata: %v", err)
	}
	cleanupCtx, cancel := context.WithCancel(context.Background())
	mgr.SetRepositoryProvider(&fakeRepoProvider{repo: &Repository{ID: wt.RepositoryID, CleanupScript: "true"}})
	mgr.SetScriptMessageHandler(&cancellingCleanupScriptHandler{cancel: cancel})
	if err := mgr.CleanupWorktrees(cleanupCtx, []*Worktree{wt}); err == nil {
		t.Fatal("cleanup cancelled after audit returned nil")
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
	assertCleanupBranchPresent(t, wt.RepositoryPath, wt.Branch)

	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("retry partial cleanup: %v", err)
	}
	assertNoCleanupRegistration(t, wt.RepositoryPath, wt.Path)
	assertCleanupBranchAbsent(t, wt.RepositoryPath, wt.Branch)
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
}

func TestCleanupWorktrees_IsIdempotentAfterVerifiedRemoval(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-idempotent", "session-idempotent", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-idempotent", "session-idempotent")
	wt.CleanupHeadOID = strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))

	for attempt := 1; attempt <= 2; attempt++ {
		if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err != nil {
			t.Fatalf("cleanup attempt %d: %v", attempt, err)
		}
	}
	assertNoCleanupRegistration(t, wt.RepositoryPath, wt.Path)
	assertCleanupBranchAbsent(t, wt.RepositoryPath, wt.Branch)
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
}

func TestCleanupWorktrees_RefusesUniqueWork(t *testing.T) {
	tests := []struct {
		name          string
		add           func(*testing.T, *Worktree)
		expectRemoval bool
	}{
		{
			name:          "unmerged commit",
			expectRemoval: true,
			add: func(t *testing.T, wt *Worktree) {
				t.Helper()
				runGit(t, wt.Path, "commit", "--allow-empty", "-m", "unique local work")
			},
		},
		{
			name: "untracked file",
			add: func(t *testing.T, wt *Worktree) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(wt.Path, "unique.txt"), []byte("keep me\n"), 0o644); err != nil {
					t.Fatalf("write unique file: %v", err)
				}
			},
		},
		{
			name: "tracked modification",
			add: func(t *testing.T, wt *Worktree) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(wt.Path, "README.md"), []byte("keep this edit\n"), 0o644); err != nil {
					t.Fatalf("modify tracked file: %v", err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mgr, store := newReferenceCleanupTestManager(t)
			taskID := "task-unique-" + strings.ReplaceAll(test.name, " ", "-")
			sessionID := "session-unique-" + strings.ReplaceAll(test.name, " ", "-")
			seedReferenceCleanupSession(t, store, taskID, sessionID, models.TaskSessionStateCompleted)
			wt := createReferenceCleanupWorktree(t, mgr, taskID, sessionID)
			test.add(t, wt)

			err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt})
			assertCleanupBranchPresent(t, wt.RepositoryPath, wt.Branch)
			if test.expectRemoval {
				if err != nil {
					t.Fatalf("cleanup preserving unique commit: %v", err)
				}
				if _, statErr := os.Lstat(wt.Path); !os.IsNotExist(statErr) {
					t.Fatalf("clean worktree path remains after branch preservation: %v", statErr)
				}
				assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
				return
			}
			if err == nil {
				t.Fatal("cleanup of uncommitted work returned nil")
			}
			if _, statErr := os.Lstat(wt.Path); statErr != nil {
				t.Fatalf("worktree containing untracked work was removed: %v", statErr)
			}
			assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
		})
	}
}

func TestCleanupWorktrees_PreservesUnrelatedWorktreeAndBranch(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-target", "session-target", models.TaskSessionStateCompleted)
	target := createReferenceCleanupWorktree(t, mgr, "task-target", "session-target")

	unrelatedPath := filepath.Join(t.TempDir(), "unrelated-worktree")
	runGit(t, target.RepositoryPath, "worktree", "add", "-b", "feature/unrelated", unrelatedPath, "main")
	runGit(t, unrelatedPath, "commit", "--allow-empty", "-m", "unrelated unique work")
	unrelatedHead := strings.TrimSpace(runGit(t, unrelatedPath, "rev-parse", "HEAD"))

	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{target}); err != nil {
		t.Fatalf("cleanup target worktree: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, unrelatedPath, "rev-parse", "HEAD")); got != unrelatedHead {
		t.Fatalf("unrelated worktree HEAD = %q, want %q", got, unrelatedHead)
	}
	assertCleanupBranchPresent(t, target.RepositoryPath, "feature/unrelated")
	assertWorktreeReferenceStatus(t, store, target.ID, StatusDeleted)
}

func TestCleanupWorktrees_RefusesUnrelatedReplacementAtRecordedPath(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-replaced", "session-replaced", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-replaced", "session-replaced")

	runGit(t, wt.RepositoryPath, "worktree", "remove", "--force", wt.Path)
	if err := os.MkdirAll(wt.Path, 0o755); err != nil {
		t.Fatalf("create replacement directory: %v", err)
	}
	sentinel := filepath.Join(wt.Path, "unrelated.txt")
	if err := os.WriteFile(sentinel, []byte("unrelated\n"), 0o644); err != nil {
		t.Fatalf("write replacement sentinel: %v", err)
	}

	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err == nil {
		t.Fatal("cleanup of unrelated replacement returned nil")
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "unrelated\n" {
		t.Fatalf("replacement sentinel changed: contents=%q err=%v", got, err)
	}
	assertCleanupBranchPresent(t, wt.RepositoryPath, wt.Branch)
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
}

func TestCleanupWorktrees_AuditsBeforeCleanupScript(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-script-order", "session-script-order", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-script-order", "session-script-order")
	if err := os.WriteFile(filepath.Join(wt.Path, "README.md"), []byte("keep this edit\n"), 0o644); err != nil {
		t.Fatalf("modify tracked file: %v", err)
	}

	provider := &fakeRepoProvider{repo: &Repository{ID: wt.RepositoryID, CleanupScript: "git clean -fd"}}
	handler := &cleanupOrderRecordingScriptHandler{}
	mgr.SetRepositoryProvider(provider)
	mgr.SetScriptMessageHandler(handler)

	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err == nil {
		t.Fatal("cleanup of dirty worktree returned nil")
	}
	if handler.cleanupRuns != 0 {
		t.Fatalf("cleanup script ran %d times before the audit rejected the worktree, want 0", handler.cleanupRuns)
	}
	if _, err := os.Stat(filepath.Join(wt.Path, "README.md")); err != nil {
		t.Fatalf("dirty worktree was changed before audit: %v", err)
	}
}

func TestCleanupWorktrees_RejectsChangedHeadFromCleanupSnapshot(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-head-snapshot", "session-head-snapshot", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-head-snapshot", "session-head-snapshot")
	wt.CleanupHeadOID = strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	runGit(t, wt.Path, "commit", "--allow-empty", "-m", "advance cleanup branch")

	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err == nil {
		t.Fatal("cleanup accepted a worktree whose HEAD changed after the snapshot")
	}
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("changed worktree path was removed: %v", err)
	}
	assertCleanupBranchPresent(t, wt.RepositoryPath, wt.Branch)
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
}

func TestCleanupWorktrees_AllowsMissingBranchMetadataForOwnedPath(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-detached-metadata", "session-detached-metadata", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-detached-metadata", "session-detached-metadata")
	wt.CleanupHeadOID = strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	wt.Branch = ""

	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("cleanup with missing branch metadata: %v", err)
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("owned path remains after branchless cleanup: %v", err)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
}

func TestCleanupWorktrees_BatchEnrichesCachedRepositoryPath(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-batch-cache", "session-batch-cache", models.TaskSessionStateCompleted)
	created := createReferenceCleanupWorktree(t, mgr, "task-batch-cache", "session-batch-cache")
	persisted := *created
	persisted.RepositoryPath = ""
	persisted.BaseBranch = ""

	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{&persisted}); err != nil {
		t.Fatalf("batch cleanup with an incomplete store projection: %v", err)
	}
	assertNoCleanupRegistration(t, created.RepositoryPath, created.Path)
	assertCleanupBranchAbsent(t, created.RepositoryPath, created.Branch)
	assertWorktreeReferenceStatus(t, store, created.ID, StatusDeleted)
}

func assertNoCleanupRegistration(t *testing.T, repoPath, worktreePath string) {
	t.Helper()
	out := runGit(t, repoPath, "worktree", "list", "--porcelain")
	want, err := normalizedWorktreeTargetPath(worktreePath)
	if err != nil {
		t.Fatalf("normalize expected worktree path: %v", err)
	}
	for _, registration := range parseWorktreeRegistrations(strings.ReplaceAll(out, "\n", "\x00")) {
		got, normalizeErr := normalizedWorktreeTargetPath(registration.path)
		if normalizeErr != nil {
			t.Fatalf("normalize registered worktree path %q: %v", registration.path, normalizeErr)
		}
		if got == want {
			t.Fatalf("worktree registration remains for %q:\n%s", worktreePath, out)
		}
	}
}

func assertCleanupBranchPresent(t *testing.T, repoPath, branch string) {
	t.Helper()
	if got := strings.TrimSpace(runGit(t, repoPath, "branch", "--list", branch)); got == "" {
		t.Fatalf("branch %q was deleted", branch)
	}
}

func assertCleanupBranchAbsent(t *testing.T, repoPath, branch string) {
	t.Helper()
	if got := strings.TrimSpace(runGit(t, repoPath, "branch", "--list", branch)); got != "" {
		t.Fatalf("branch %q remains: %q", branch, got)
	}
}
