package lifecycle

import (
	"context"
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
	return NewWorktreePreparer(mgr, log), mgr
}

func TestWorktreePreparer_MultiRepo_CreatesWorktreePerRepo(t *testing.T) {
	repoA := initBareGitRepo(t, "frontend")
	repoB := initBareGitRepo(t, "backend")

	preparer, _ := newPreparerForTest(t)

	req := &EnvPrepareRequest{
		TaskID:       "task-multi-1",
		SessionID:    "sess-multi-1",
		TaskTitle:    "Multi Repo Task",
		ExecutorType: executor.Name("worktree"),
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
		ExecutorType: executor.Name("worktree"),
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

func TestWorktreePreparer_MultiRepo_RunsPerRepoSetupScript(t *testing.T) {
	repoA := initBareGitRepo(t, "frontend")
	repoB := initBareGitRepo(t, "backend")

	preparer, _ := newPreparerForTest(t)

	req := &EnvPrepareRequest{
		TaskID:       "task-multi-setup",
		SessionID:    "sess-multi-setup",
		TaskTitle:    "Setup Task",
		ExecutorType: executor.Name("worktree"),
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
	// Each repo should produce a "Run setup script (<name>)" step.
	var setupStepsSeen int
	for _, s := range res.Steps {
		if strings.HasPrefix(s.Name, "Run setup script") {
			setupStepsSeen++
		}
	}
	if setupStepsSeen != 2 {
		t.Errorf("expected 2 per-repo setup script steps, got %d", setupStepsSeen)
	}
}
