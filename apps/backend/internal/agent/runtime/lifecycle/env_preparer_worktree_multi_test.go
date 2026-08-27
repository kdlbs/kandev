package lifecycle

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/worktree"
)

// inMemoryWorktreeStore is a minimal worktree.Store for preparer tests.
// Mirrors the package-internal mockStore, but lives here so tests don't need
// to import worktree-internal symbols.
type inMemoryWorktreeStore struct {
	mu        sync.Mutex
	worktrees map[string]*worktree.Worktree
}

func newInMemoryWorktreeStore() *inMemoryWorktreeStore {
	return &inMemoryWorktreeStore{worktrees: make(map[string]*worktree.Worktree)}
}

func (s *inMemoryWorktreeStore) CreateWorktree(_ context.Context, wt *worktree.Worktree) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.worktrees[wt.ID] = wt
	return nil
}

func (s *inMemoryWorktreeStore) GetWorktreeByID(_ context.Context, id string) (*worktree.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wt, ok := s.worktrees[id]
	if !ok {
		return nil, nil
	}
	return wt, nil
}

func (s *inMemoryWorktreeStore) GetWorktreeBySessionID(_ context.Context, sessionID string) (*worktree.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, wt := range s.worktrees {
		if wt.SessionID == sessionID {
			return wt, nil
		}
	}
	return nil, nil
}

func (s *inMemoryWorktreeStore) GetWorktreesByTaskID(_ context.Context, taskID string) ([]*worktree.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*worktree.Worktree
	for _, wt := range s.worktrees {
		if wt.TaskID == taskID {
			out = append(out, wt)
		}
	}
	return out, nil
}

func (s *inMemoryWorktreeStore) GetWorktreesByRepositoryID(_ context.Context, repoID string) ([]*worktree.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*worktree.Worktree
	for _, wt := range s.worktrees {
		if wt.RepositoryID == repoID {
			out = append(out, wt)
		}
	}
	return out, nil
}

func (s *inMemoryWorktreeStore) UpdateWorktree(_ context.Context, wt *worktree.Worktree) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.worktrees[wt.ID] = wt
	return nil
}

func (s *inMemoryWorktreeStore) DeleteWorktree(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.worktrees, id)
	return nil
}

func (s *inMemoryWorktreeStore) ListActiveWorktrees(_ context.Context) ([]*worktree.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*worktree.Worktree
	for _, wt := range s.worktrees {
		if wt.Status == worktree.StatusActive {
			out = append(out, wt)
		}
	}
	return out, nil
}

func (s *inMemoryWorktreeStore) ListActiveWorktreePaths(_ context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var paths []string
	for _, wt := range s.worktrees {
		if wt.Status == worktree.StatusActive && wt.Path != "" {
			paths = append(paths, wt.Path)
		}
	}
	return paths, nil
}

func (s *inMemoryWorktreeStore) CountActiveWorktreeReferences(_ context.Context, _ string, _ []string) (int, error) {
	return 0, nil
}

// GetWorktreesBySessionID — MultiRepoStore.
func (s *inMemoryWorktreeStore) GetWorktreesBySessionID(_ context.Context, sessionID string) ([]*worktree.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*worktree.Worktree
	for _, wt := range s.worktrees {
		if wt.SessionID == sessionID && wt.Status == worktree.StatusActive {
			out = append(out, wt)
		}
	}
	return out, nil
}

// GetWorktreeBySessionAndRepository — MultiRepoStore.
func (s *inMemoryWorktreeStore) GetWorktreeBySessionAndRepository(_ context.Context, sessionID, repoID string) (*worktree.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, wt := range s.worktrees {
		if wt.SessionID == sessionID && wt.RepositoryID == repoID && wt.Status == worktree.StatusActive {
			return wt, nil
		}
	}
	return nil, nil
}

// initBareGitRepo creates a single-commit git repo in a temp directory and
// returns its path. Used as a stand-in for a "real" repository in WorktreePreparer
// multi-repo tests.
func initBareGitRepo(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s in %s failed: %v\n%s", strings.Join(args, " "), dir, err, out)
		}
	}
	mustGit("init", "-b", "main")
	mustGit("config", "user.email", "test@example.com")
	mustGit("config", "user.name", "Test User")
	mustGit("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello "+name), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	mustGit("add", "README.md")
	mustGit("commit", "-m", "initial commit")
	return dir
}

func newPreparerForTest(t *testing.T) (*WorktreePreparer, *worktree.Manager) {
	t.Helper()
	preparer, mgr, _ := newPreparerForTestWithStore(t)
	return preparer, mgr
}

func newPreparerForTestWithStore(t *testing.T) (*WorktreePreparer, *worktree.Manager, *inMemoryWorktreeStore) {
	t.Helper()
	tmp := t.TempDir()
	cfg := worktree.Config{
		Enabled:       true,
		TasksBasePath: filepath.Join(tmp, "tasks"),
		BranchPrefix:  "feat/",
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	store := newInMemoryWorktreeStore()
	mgr, err := worktree.NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("worktree manager: %v", err)
	}
	return NewWorktreePreparer(mgr, log), mgr, store
}

func TestWorktreePreparer_MultiRepo_CreatesWorktreePerRepo(t *testing.T) {
	repoA := initBareGitRepo(t, "frontend")
	repoB := initBareGitRepo(t, "backend")

	preparer, _ := newPreparerForTest(t)

	req := &EnvPrepareRequest{
		TaskID:       "task-multi-1",
		SessionID:    "sess-multi-1",
		TaskTitle:    "Multi Repo Task",
		ExecutorType: executor.NameStandalone,
		TaskDirName:  "multi-repo-task_aaa",
		Repositories: []RepoPrepareSpec{
			{RepositoryID: "repo-front", RepositoryPath: repoA, RepoName: "frontend", BaseBranch: "main"},
			{RepositoryID: "repo-back", RepositoryPath: repoB, RepoName: "backend", BaseBranch: "main"},
		},
	}

	res, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success; steps: %+v err: %s", res.Steps, res.ErrorMessage)
	}
	if len(res.Worktrees) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(res.Worktrees))
	}
	if res.Worktrees[0].RepositoryID != "repo-front" || res.Worktrees[1].RepositoryID != "repo-back" {
		t.Errorf("unexpected order: %+v", res.Worktrees)
	}
	// Legacy single-worktree fields mirror Worktrees[0].
	if res.WorktreeID != res.Worktrees[0].WorktreeID {
		t.Errorf("legacy WorktreeID should mirror Worktrees[0]; got %q vs %q", res.WorktreeID, res.Worktrees[0].WorktreeID)
	}
	// WorkspacePath = parent of any repo subdir (the task root).
	if res.WorkspacePath == "" {
		t.Error("expected workspace_path to be set")
	}
	// Both worktrees should live under the same task root.
	if filepath.Dir(res.Worktrees[0].WorktreePath) != filepath.Dir(res.Worktrees[1].WorktreePath) {
		t.Errorf("worktrees should share task root: %s vs %s",
			res.Worktrees[0].WorktreePath, res.Worktrees[1].WorktreePath)
	}
	// Each worktree directory must exist on disk.
	for _, w := range res.Worktrees {
		if _, err := os.Stat(w.WorktreePath); err != nil {
			t.Errorf("worktree dir missing for %s: %v", w.RepositoryID, err)
		}
	}
}

func TestWorktreePreparer_MultiRepo_RollbackOnPartialFailure(t *testing.T) {
	repoA := initBareGitRepo(t, "good")
	// repoB is intentionally a non-git directory to force the second create to fail.
	repoB := t.TempDir()

	preparer, mgr := newPreparerForTest(t)

	req := &EnvPrepareRequest{
		TaskID:       "task-multi-fail",
		SessionID:    "sess-multi-fail",
		TaskTitle:    "Failing Task",
		ExecutorType: executor.NameStandalone,
		TaskDirName:  "failing-task_bbb",
		Repositories: []RepoPrepareSpec{
			{RepositoryID: "repo-good", RepositoryPath: repoA, RepoName: "good", BaseBranch: "main"},
			{RepositoryID: "repo-bad", RepositoryPath: repoB, RepoName: "bad", BaseBranch: "main"},
		},
	}

	res, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("prepare returned error (should report via result): %v", err)
	}
	if res.Success {
		t.Fatal("expected failure when second repo is not a git repo")
	}
	if res.ErrorMessage == "" {
		t.Error("expected non-empty error message")
	}
	if res.Error == nil {
		t.Error("expected failed multi-repo result to retain its error cause")
	}

	// Rollback: no worktrees should remain in the store after partial failure.
	all, err := mgr.GetAllByTaskID(context.Background(), "task-multi-fail")
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	for _, wt := range all {
		if wt.Status == worktree.StatusActive {
			t.Errorf("expected no active worktrees after rollback; found %s in %s", wt.ID, wt.Path)
		}
	}
}

func TestWorktreePreparer_MultiRepo_RequiredRefreshIdentifiesFailingRepository(t *testing.T) {
	repoA := initBareGitRepo(t, "good-refresh")
	repoB := initBareGitRepo(t, "bad-refresh")

	preparer, mgr := newPreparerForTest(t)
	req := &EnvPrepareRequest{
		TaskID:       "task-refresh-fail",
		SessionID:    "sess-refresh-fail",
		TaskTitle:    "Required refresh failure",
		ExecutorType: executor.NameStandalone,
		TaskDirName:  "refresh-fail_eee",
		Repositories: []RepoPrepareSpec{
			{TaskRepositoryID: "tr-good", RepositoryID: "repo-good", RepositoryPath: repoA, RepoName: "good", BaseBranch: "main"},
			{TaskRepositoryID: "tr-bad", RepositoryID: "repo-bad", RepositoryPath: repoB, RepoName: "backend", BaseBranch: "main", PullBeforeWorktree: true},
		},
	}

	res, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("prepare returned hard error: %v", err)
	}
	if res.Success {
		t.Fatal("required refresh failure unexpectedly succeeded")
	}
	if !strings.Contains(res.ErrorMessage, "repo-bad") || !strings.Contains(res.ErrorMessage, "backend") {
		t.Fatalf("error = %q, want failing repository identity and name", res.ErrorMessage)
	}
	var preparationErr *RepositoryPreparationError
	if !errors.As(res.Error, &preparationErr) {
		t.Fatalf("error = %T %v, want RepositoryPreparationError", res.Error, res.Error)
	}
	if preparationErr.RepositoryID != "repo-bad" || preparationErr.TaskRepositoryID != "tr-bad" {
		t.Fatalf("preparation error identity = %+v, want repo-bad/tr-bad", preparationErr)
	}
	if all, listErr := mgr.GetAllByTaskID(context.Background(), "task-refresh-fail"); listErr != nil {
		t.Fatalf("list worktrees: %v", listErr)
	} else {
		for _, wt := range all {
			if wt.Status == worktree.StatusActive {
				t.Fatalf("active worktree after required refresh failure = %s, want none", wt.ID)
			}
		}
	}
}

func TestWorktreePreparer_MultiRepo_RollbackKeepsReusedWorktrees(t *testing.T) {
	repoExisting := initBareGitRepo(t, "existing")
	repoNew := initBareGitRepo(t, "new")
	repoBad := t.TempDir()

	preparer, mgr, store := newPreparerForTestWithStore(t)
	existingPath := filepath.Join(t.TempDir(), "existing-worktree")
	if err := os.MkdirAll(existingPath, 0755); err != nil {
		t.Fatalf("mkdir existing worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(existingPath, ".git"), []byte("gitdir: /tmp/existing.git\n"), 0644); err != nil {
		t.Fatalf("write existing .git file: %v", err)
	}
	if err := store.CreateWorktree(context.Background(), &worktree.Worktree{
		ID:           "wt-existing",
		SessionID:    "source-session",
		TaskID:       "task-reuse-fail",
		RepositoryID: "repo-existing",
		BranchSlug:   "main",
		Path:         existingPath,
		Status:       worktree.StatusActive,
	}); err != nil {
		t.Fatalf("seed existing worktree: %v", err)
	}

	req := &EnvPrepareRequest{
		TaskID:       "task-reuse-fail",
		SessionID:    "sess-reuse-fail",
		TaskTitle:    "Reuse Then Fail",
		ExecutorType: executor.NameStandalone,
		TaskDirName:  "reuse-fail_ccc",
		Repositories: []RepoPrepareSpec{
			{
				RepositoryID:       "repo-existing",
				RepositoryPath:     repoExisting,
				RepoName:           "existing",
				BaseBranch:         "main",
				WorktreeID:         "wt-existing",
				BranchIdentitySlug: "main",
			},
			{RepositoryID: "repo-new", RepositoryPath: repoNew, RepoName: "new", BaseBranch: "main"},
			{RepositoryID: "repo-bad", RepositoryPath: repoBad, RepoName: "bad", BaseBranch: "main"},
		},
	}

	res, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("prepare returned hard error: %v", err)
	}
	if res.Success {
		t.Fatal("expected failure when final repo is not a git repo")
	}

	existing, err := store.GetWorktreeByID(context.Background(), "wt-existing")
	if err != nil {
		t.Fatalf("get existing worktree: %v", err)
	}
	if existing == nil || existing.Status != worktree.StatusActive {
		t.Fatalf("reused worktree was rolled back: %+v", existing)
	}
	if !mgr.IsValid(existingPath) {
		t.Fatalf("reused worktree path should remain valid: %s", existingPath)
	}

	all, err := mgr.GetAllByTaskID(context.Background(), "task-reuse-fail")
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	for _, wt := range all {
		if wt.ID != "wt-existing" && wt.Status == worktree.StatusActive {
			t.Fatalf("new worktree %s should have been rolled back", wt.ID)
		}
	}
}

func TestWorktreePreparer_MultiRepo_RollbackRemovesWorktreeCreatedForStaleReuseID(t *testing.T) {
	repoNew := initBareGitRepo(t, "stale-new")
	repoBad := t.TempDir()

	preparer, mgr := newPreparerForTest(t)
	req := &EnvPrepareRequest{
		TaskID:       "task-stale-reuse-fail",
		SessionID:    "sess-stale-reuse-fail",
		TaskTitle:    "Stale Reuse Then Fail",
		ExecutorType: executor.NameStandalone,
		TaskDirName:  "stale-reuse-fail_ddd",
		Repositories: []RepoPrepareSpec{
			{
				RepositoryID:   "repo-new",
				RepositoryPath: repoNew,
				RepoName:       "stale-new",
				BaseBranch:     "main",
				WorktreeID:     "wt-stale",
			},
			{RepositoryID: "repo-bad", RepositoryPath: repoBad, RepoName: "bad", BaseBranch: "main"},
		},
	}

	res, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("prepare returned hard error: %v", err)
	}
	if res.Success {
		t.Fatal("expected failure when final repo is not a git repo")
	}

	all, err := mgr.GetAllByTaskID(context.Background(), "task-stale-reuse-fail")
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	for _, wt := range all {
		if wt.Status == worktree.StatusActive {
			t.Fatalf("worktree %s should have been rolled back after stale reuse ID created it", wt.ID)
		}
	}
}

// TestWorktreePreparer_FreshStartRejectsStaleWorktreePathOwnedByLiveTask
// protects the fresh-start path from a stale worktree record whose on-disk
// path has since been claimed by another live task. The stale task must fail
// safely; it must never attach to or change the live task's checkout.
func TestWorktreePreparer_FreshStartRejectsStaleWorktreePathOwnedByLiveTask(t *testing.T) {
	repositoryPath := initBareGitRepo(t, "shared-repository")
	preparer, manager, store := newPreparerForTestWithStore(t)
	ctx := context.Background()

	live, err := manager.Create(ctx, worktree.CreateRequest{
		TaskID:         "task-live",
		SessionID:      "session-live",
		TaskTitle:      "Live task",
		RepositoryID:   "repository-shared",
		RepositoryPath: repositoryPath,
		BaseBranch:     "main",
		TaskDirName:    "live-task_abc",
		RepoName:       "shared-repository",
	})
	if err != nil {
		t.Fatalf("create live worktree: %v", err)
	}
	liveHead := strings.TrimSpace(runGitForPreparerTest(t, live.Path, "rev-parse", "HEAD"))

	if err := store.CreateWorktree(ctx, &worktree.Worktree{
		ID:           "worktree-stale",
		TaskID:       "task-stale",
		SessionID:    "session-stale",
		RepositoryID: "repository-shared",
		Path:         live.Path,
		Branch:       "feat/stale",
		Status:       worktree.StatusActive,
	}); err != nil {
		t.Fatalf("seed stale worktree: %v", err)
	}

	result, err := preparer.Prepare(ctx, &EnvPrepareRequest{
		TaskID:         "task-stale",
		SessionID:      "session-stale",
		TaskTitle:      "Stale task",
		ExecutorType:   executor.NameStandalone,
		RepositoryID:   "repository-shared",
		RepositoryPath: repositoryPath,
		BaseBranch:     "main",
		WorktreeID:     "worktree-stale",
		TaskDirName:    "stale-task_def",
		RepoName:       "shared-repository",
	}, nil)
	if err != nil {
		t.Fatalf("prepare returned hard error: %v", err)
	}
	if result.Success {
		t.Fatal("fresh start succeeded by reusing a live task's worktree path")
	}
	if !errors.Is(result.Error, worktree.ErrWorktreePathOwnedByAnotherTask) {
		t.Fatalf("prepare error = %v, want ErrWorktreePathOwnedByAnotherTask", result.Error)
	}
	if got := strings.TrimSpace(runGitForPreparerTest(t, live.Path, "rev-parse", "HEAD")); got != liveHead {
		t.Fatalf("live worktree HEAD changed: got %q, want %q", got, liveHead)
	}
	if !manager.IsValid(live.Path) {
		t.Fatal("live worktree was removed or corrupted")
	}
}

// TestWorktreePreparer_FreshStartRejectsStaleWorktreePathOwnedByLiveTask_WorktreeIDOnly
// exercises the WorktreeID-only lookup branch of tryReuseExisting (when
// SessionID is empty), confirming the ownership check fires there too.
func TestWorktreePreparer_FreshStartRejectsStaleWorktreePathOwnedByLiveTask_WorktreeIDOnly(t *testing.T) {
	repositoryPath := initBareGitRepo(t, "shared-repository")
	preparer, manager, store := newPreparerForTestWithStore(t)
	ctx := context.Background()

	live, err := manager.Create(ctx, worktree.CreateRequest{
		TaskID:         "task-live",
		SessionID:      "session-live",
		TaskTitle:      "Live task",
		RepositoryID:   "repository-shared",
		RepositoryPath: repositoryPath,
		BaseBranch:     "main",
		TaskDirName:    "live-task_wtid",
		RepoName:       "shared-repository",
	})
	if err != nil {
		t.Fatalf("create live worktree: %v", err)
	}
	liveHead := strings.TrimSpace(runGitForPreparerTest(t, live.Path, "rev-parse", "HEAD"))

	if err := store.CreateWorktree(ctx, &worktree.Worktree{
		ID:           "worktree-stale-wtid",
		TaskID:       "task-stale-wtid",
		SessionID:    "session-stale-wtid",
		RepositoryID: "repository-shared",
		Path:         live.Path,
		Branch:       "feat/stale-wtid",
		Status:       worktree.StatusActive,
	}); err != nil {
		t.Fatalf("seed stale worktree: %v", err)
	}

	result, err := preparer.Prepare(ctx, &EnvPrepareRequest{
		TaskID:         "task-stale-wtid",
		SessionID:      "", // empty — forces WorktreeID-only lookup
		TaskTitle:      "Stale task WTID-only",
		ExecutorType:   executor.NameStandalone,
		RepositoryID:   "repository-shared",
		RepositoryPath: repositoryPath,
		BaseBranch:     "main",
		WorktreeID:     "worktree-stale-wtid",
		TaskDirName:    "stale-task-wtid_xyz",
		RepoName:       "shared-repository",
	}, nil)
	if err != nil {
		t.Fatalf("prepare returned hard error: %v", err)
	}
	if result.Success {
		t.Fatal("fresh start succeeded by reusing a live task's worktree path via WorktreeID-only lookup")
	}
	if !errors.Is(result.Error, worktree.ErrWorktreePathOwnedByAnotherTask) {
		t.Fatalf("prepare error = %v, want ErrWorktreePathOwnedByAnotherTask", result.Error)
	}
	if got := strings.TrimSpace(runGitForPreparerTest(t, live.Path, "rev-parse", "HEAD")); got != liveHead {
		t.Fatalf("live worktree HEAD changed: got %q, want %q", got, liveHead)
	}
	if !manager.IsValid(live.Path) {
		t.Fatal("live worktree was removed or corrupted")
	}
}

func runGitForPreparerTest(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s failed: %v\n%s", strings.Join(args, " "), directory, err, output)
	}
	return string(output)
}

func TestWorktreePreparer_MultiRepo_RunsPerRepoSetupScript(t *testing.T) {
	repoA := initBareGitRepo(t, "frontend")
	repoB := initBareGitRepo(t, "backend")

	// Per-repo setup scripts are executed by the worktree manager during
	// Create() through its script handler. Wire one up so the scripts run.
	repos := map[string]*worktree.Repository{
		"repo-front": {ID: "repo-front", SetupScript: "echo front-script-ran > setup-marker.txt"},
		"repo-back":  {ID: "repo-back", SetupScript: "echo back-script-ran > setup-marker.txt"},
	}
	preparer, _, _ := newPreparerWithScriptHandler(t, repos)

	req := &EnvPrepareRequest{
		TaskID:       "task-multi-setup",
		SessionID:    "sess-multi-setup",
		TaskTitle:    "Setup Task",
		ExecutorType: executor.NameStandalone,
		TaskDirName:  "setup-task_ccc",
		Repositories: []RepoPrepareSpec{
			{
				RepositoryID:    "repo-front",
				RepositoryPath:  repoA,
				RepoName:        "frontend",
				BaseBranch:      "main",
				RepoSetupScript: "echo front-script-ran > setup-marker.txt",
			},
			{
				RepositoryID:    "repo-back",
				RepositoryPath:  repoB,
				RepoName:        "backend",
				BaseBranch:      "main",
				RepoSetupScript: "echo back-script-ran > setup-marker.txt",
			},
		},
	}

	res, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success; err: %s", res.ErrorMessage)
	}
	if len(res.Worktrees) != 2 {
		t.Fatalf("expected 2 worktrees; got %d", len(res.Worktrees))
	}
	for i, w := range res.Worktrees {
		marker := filepath.Join(w.WorktreePath, "setup-marker.txt")
		if _, err := os.Stat(marker); err != nil {
			t.Errorf("repo %d setup marker missing: %v", i, err)
		}
	}
}

// TestWorktreePreparer_FreshStartRejectsStaleWorktreePath_NestedProjectMarker
// protects against the ownership-marker walk being confused by a legitimate
// .kandev-workspace.json file inside a descendant repository checkout. The
// stale task must still be rejected — the presence of a non-task-root marker
// must not shadow the task-root marker.
func TestWorktreePreparer_FreshStartRejectsStaleWorktreePath_NestedProjectMarker(t *testing.T) {
	repositoryPath := initBareGitRepo(t, "repo-with-marker")
	preparer, manager, store := newPreparerForTestWithStore(t)
	ctx := context.Background()

	live, err := manager.Create(ctx, worktree.CreateRequest{
		TaskID:         "task-live-nested",
		SessionID:      "session-live-nested",
		TaskTitle:      "Live task",
		RepositoryID:   "repository-nested",
		RepositoryPath: repositoryPath,
		BaseBranch:     "main",
		TaskDirName:    "live-task_nested",
		RepoName:       "repo-with-marker",
	})
	if err != nil {
		t.Fatalf("create live worktree: %v", err)
	}
	liveHead := strings.TrimSpace(runGitForPreparerTest(t, live.Path, "rev-parse", "HEAD"))

	// Write a legitimate .kandev-workspace.json inside the repo checkout itself
	// (as a real project might) to see if it shadows the task-root marker.
	if err := os.WriteFile(filepath.Join(live.Path, ".kandev-workspace.json"),
		[]byte(`{"project":"not-kandev","version":1}`), 0644); err != nil {
		t.Fatalf("write project marker: %v", err)
	}

	if err := store.CreateWorktree(ctx, &worktree.Worktree{
		ID:           "worktree-stale-nested",
		TaskID:       "task-stale-nested",
		SessionID:    "session-stale-nested",
		RepositoryID: "repository-nested",
		Path:         live.Path,
		Branch:       "feat/stale-nested",
		Status:       worktree.StatusActive,
	}); err != nil {
		t.Fatalf("seed stale worktree: %v", err)
	}

	result, err := preparer.Prepare(ctx, &EnvPrepareRequest{
		TaskID:         "task-stale-nested",
		SessionID:      "", // empty — forces WorktreeID-only lookup
		TaskTitle:      "Stale task nested-marker",
		ExecutorType:   executor.NameStandalone,
		RepositoryID:   "repository-nested",
		RepositoryPath: repositoryPath,
		BaseBranch:     "main",
		WorktreeID:     "worktree-stale-nested",
		TaskDirName:    "stale-task-nested_def",
		RepoName:       "repo-with-marker",
	}, nil)
	if err != nil {
		t.Fatalf("prepare returned hard error: %v", err)
	}
	if result.Success {
		t.Fatal("fresh start succeeded despite nested non-ownership marker in repo checkout")
	}
	if !errors.Is(result.Error, worktree.ErrWorktreePathOwnedByAnotherTask) {
		t.Fatalf("prepare error = %v, want ErrWorktreePathOwnedByAnotherTask", result.Error)
	}
	if got := strings.TrimSpace(runGitForPreparerTest(t, live.Path, "rev-parse", "HEAD")); got != liveHead {
		t.Fatalf("live worktree HEAD changed: got %q, want %q", got, liveHead)
	}
	if !manager.IsValid(live.Path) {
		t.Fatal("live worktree was removed or corrupted")
	}
}
