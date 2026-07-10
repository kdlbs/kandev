package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fakeRepoProvider lets tests inject a Repository (with CopyFiles) without
// pulling in the full task service / models layer.
type fakeRepoProvider struct {
	repo *Repository
	err  error
}

func (f *fakeRepoProvider) GetRepository(_ context.Context, _ string) (*Repository, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.repo, nil
}

func (f *fakeRepoProvider) GetRepositoryByPath(_ context.Context, _ string) (*Repository, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.repo, nil
}

// writeSourceFile creates a file (and any parent dirs) inside repoPath with
// the given content. Returned path is the absolute file path.
func writeSourceFile(t *testing.T, repoPath, rel, content string) string {
	t.Helper()
	abs := filepath.Join(repoPath, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return abs
}

func newManagerForCopyTest(t *testing.T, provider RepositoryProvider) *Manager {
	t.Helper()
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	if provider != nil {
		mgr.SetRepositoryProvider(provider)
	}
	return mgr
}

func createReqForCopyTest(repoPath, name string) CreateRequest {
	return CreateRequest{
		TaskID:         "task-" + name,
		SessionID:      "session-" + name,
		TaskTitle:      "Copy Files Test",
		RepositoryID:   "repo-" + name,
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		TaskDirName:    "task-" + name,
		RepoName:       "repo-" + name,
	}
}

func TestManagerCreate_CopiesFiles_HappyPath(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	writeSourceFile(t, repoPath, ".env", "SECRET=hunter2\n")
	writeSourceFile(t, repoPath, "config/local.yml", "debug: true\n")

	provider := &fakeRepoProvider{
		repo: &Repository{ID: "repo-happy", CopyFiles: ".env, config/local.yml"},
	}
	mgr := newManagerForCopyTest(t, provider)

	wt, err := mgr.Create(context.Background(), createReqForCopyTest(repoPath, "happy"))
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	envBytes, err := os.ReadFile(filepath.Join(wt.Path, ".env"))
	if err != nil {
		t.Fatalf("expected .env in worktree: %v", err)
	}
	if string(envBytes) != "SECRET=hunter2\n" {
		t.Fatalf(".env content = %q, want %q", string(envBytes), "SECRET=hunter2\n")
	}

	cfgBytes, err := os.ReadFile(filepath.Join(wt.Path, "config", "local.yml"))
	if err != nil {
		t.Fatalf("expected config/local.yml in worktree: %v", err)
	}
	if string(cfgBytes) != "debug: true\n" {
		t.Fatalf("config/local.yml content = %q, want %q", string(cfgBytes), "debug: true\n")
	}
}

func TestManagerCreate_CopyFilesEmpty_NoOp(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	// Add an untracked file that would be copied if CopyFiles were set —
	// proves the no-op path doesn't sweep it in.
	writeSourceFile(t, repoPath, ".env", "should-not-copy")

	provider := &fakeRepoProvider{
		repo: &Repository{ID: "repo-empty", CopyFiles: ""},
	}
	mgr := newManagerForCopyTest(t, provider)

	wt, err := mgr.Create(context.Background(), createReqForCopyTest(repoPath, "empty"))
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(wt.Path, ".env")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected .env to be absent in worktree, stat err = %v", err)
	}
}

func TestManagerCreate_NoRepoProvider_NoOp(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)

	mgr := newManagerForCopyTest(t, nil) // no provider set

	wt, err := mgr.Create(context.Background(), createReqForCopyTest(repoPath, "noprov"))
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if wt == nil {
		t.Fatal("expected non-nil worktree")
	}
}

func TestManagerCreate_RepoProviderError_NoOp(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)

	provider := &fakeRepoProvider{err: errors.New("not found")}
	mgr := newManagerForCopyTest(t, provider)

	wt, err := mgr.Create(context.Background(), createReqForCopyTest(repoPath, "providererr"))
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if wt == nil {
		t.Fatal("expected non-nil worktree")
	}
}

func TestManagerCreate_CopiedFiles_RecordedOnWorktree(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	writeSourceFile(t, repoPath, ".env", "X=1\n")
	writeSourceFile(t, repoPath, "config/local.yml", "y\n")

	provider := &fakeRepoProvider{
		repo: &Repository{ID: "repo-list", CopyFiles: ".env, config/local.yml"},
	}
	mgr := newManagerForCopyTest(t, provider)

	wt, err := mgr.Create(context.Background(), createReqForCopyTest(repoPath, "list"))
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	got := map[string]bool{}
	for _, p := range wt.CopiedFiles {
		got[p] = true
	}
	for _, want := range []string{".env", "config/local.yml"} {
		if !got[want] {
			t.Errorf("wt.CopiedFiles missing %q, got %v", want, wt.CopiedFiles)
		}
	}
	if len(wt.CopyFilesWarnings) != 0 {
		t.Errorf("unexpected warnings: %v", wt.CopyFilesWarnings)
	}
}

func TestManagerCreate_MissingSourceFile_StillSucceeds(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	// .env intentionally not created in source repo.

	provider := &fakeRepoProvider{
		repo: &Repository{ID: "repo-missing", CopyFiles: ".env"},
	}
	mgr := newManagerForCopyTest(t, provider)

	wt, err := mgr.Create(context.Background(), createReqForCopyTest(repoPath, "missing"))
	if err != nil {
		t.Fatalf("Create() should succeed even when source file is missing, got: %v", err)
	}
	if wt == nil {
		t.Fatal("expected non-nil worktree")
	}
	// .env should not exist in the worktree either.
	if _, err := os.Stat(filepath.Join(wt.Path, ".env")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected .env to be absent in worktree, stat err = %v", err)
	}
}
