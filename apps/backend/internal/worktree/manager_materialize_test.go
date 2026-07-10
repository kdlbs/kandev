package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// fakeRepoProvider returns a fixed repository config for materialization tests.
type fakeRepoProvider struct {
	repo *Repository
	err  error
}

func (f *fakeRepoProvider) GetRepository(_ context.Context, _ string) (*Repository, error) {
	return f.repo, f.err
}

func (f *fakeRepoProvider) GetRepositoryByPath(_ context.Context, _ string) (*Repository, error) {
	return f.repo, f.err
}

func newMaterializeManager(t *testing.T, repo *Repository) *Manager {
	t.Helper()
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	mgr.SetRepositoryProvider(&fakeRepoProvider{repo: repo})
	return mgr
}

func materializeCreateRequest(repoPath string) CreateRequest {
	return CreateRequest{
		TaskID:         "task-mat",
		SessionID:      "sess-mat",
		RepositoryID:   "repo-mat",
		TaskTitle:      "materialize",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		TaskDirName:    "task-mat",
		RepoName:       "repo-mat",
	}
}

func TestManagerCreate_CopyModeMaterializesFile(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	// Gitignored shared file living in the main repo.
	if err := os.WriteFile(filepath.Join(repoPath, ".env.local"), []byte("TOKEN=abc"), 0o644); err != nil {
		t.Fatalf("write .env.local: %v", err)
	}

	mgr := newMaterializeManager(t, &Repository{
		ID:            "repo-mat",
		WorktreeFiles: []FileSpec{{Path: ".env.local", Mode: FileMaterializeCopy}},
	})

	wt, err := mgr.Create(context.Background(), materializeCreateRequest(repoPath))
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	dest := filepath.Join(wt.Path, ".env.local")
	info, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("expected .env.local in worktree: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("copy mode produced a symlink")
	}
	content, _ := os.ReadFile(dest)
	if string(content) != "TOKEN=abc" {
		t.Fatalf("content = %q", content)
	}
}

func TestManagerCreate_SymlinkModeMaterializesLink(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	if err := os.WriteFile(filepath.Join(repoPath, ".env.local"), []byte("SHARED=1"), 0o644); err != nil {
		t.Fatalf("write .env.local: %v", err)
	}

	mgr := newMaterializeManager(t, &Repository{
		ID:            "repo-mat",
		WorktreeFiles: []FileSpec{{Path: ".env.local", Mode: FileMaterializeSymlink}},
	})

	wt, err := mgr.Create(context.Background(), materializeCreateRequest(repoPath))
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	dest := filepath.Join(wt.Path, ".env.local")
	info, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("expected .env.local link in worktree: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink mode did not produce a symlink")
	}
	// Central update is reflected through the link.
	if err := os.WriteFile(filepath.Join(repoPath, ".env.local"), []byte("SHARED=2"), 0o644); err != nil {
		t.Fatalf("update central file: %v", err)
	}
	content, _ := os.ReadFile(dest)
	if string(content) != "SHARED=2" {
		t.Fatalf("link did not reflect central update: %q", content)
	}
}

// stubRepoService feeds a fixed models.Repository through the REAL
// RepositoryAdapter, exercising the production adapter → FileSpec → materialize
// chain (not the fake provider used by the other tests).
type stubRepoService struct{ repo *models.Repository }

func (s *stubRepoService) GetRepository(_ context.Context, _ string) (*models.Repository, error) {
	return s.repo, nil
}

func (s *stubRepoService) GetRepositoryByLocalPath(_ context.Context, _ string) (*models.Repository, error) {
	return s.repo, nil
}

func TestManagerCreate_RealAdapterMaterializesMixedModes(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	writeFile(t, filepath.Join(repoPath, ".env"), "SHARED=1")
	writeFile(t, filepath.Join(repoPath, "ONBOARDING.md"), "copied")

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// Exactly how cmd/kandev/worktree.go wires it in production.
	mgr.SetRepositoryProvider(NewRepositoryAdapter(&stubRepoService{repo: &models.Repository{
		ID: "repo-mat",
		WorktreeFiles: []models.WorktreeFile{
			{Path: ".env", Mode: "symlink"},
			{Path: "ONBOARDING.md", Mode: "copy"},
		},
	}}))

	wt, err := mgr.Create(context.Background(), materializeCreateRequest(repoPath))
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	env, err := os.Lstat(filepath.Join(wt.Path, ".env"))
	if err != nil {
		t.Fatalf(".env not materialized: %v", err)
	}
	if env.Mode()&os.ModeSymlink == 0 {
		t.Fatalf(".env should be a symlink")
	}
	ob, err := os.Lstat(filepath.Join(wt.Path, "ONBOARDING.md"))
	if err != nil {
		t.Fatalf("ONBOARDING.md not materialized: %v", err)
	}
	if ob.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("ONBOARDING.md should be a copy")
	}
}

// pathOnlyProvider resolves config only by path, and fails if asked by ID —
// mirroring the production launch path where CreateRequest.RepositoryID is empty
// and the repository must be found via its local path.
type pathOnlyProvider struct{ repo *Repository }

func (p *pathOnlyProvider) GetRepository(_ context.Context, _ string) (*Repository, error) {
	return nil, errors.New("GetRepository by ID must not be used when RepositoryID is empty")
}

func (p *pathOnlyProvider) GetRepositoryByPath(_ context.Context, _ string) (*Repository, error) {
	return p.repo, nil
}

// TestManagerCreate_MaterializesByPathWhenRepositoryIDMissing reproduces the
// production bug: the worktree CreateRequest arrives with an empty RepositoryID
// (only RepositoryPath is set), so materialization must resolve the repository
// config by path and still run.
func TestManagerCreate_MaterializesByPathWhenRepositoryIDMissing(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	writeFile(t, filepath.Join(repoPath, ".env.local"), "SHARED=1")

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	mgr.SetRepositoryProvider(&pathOnlyProvider{repo: &Repository{
		ID:            "resolved-by-path",
		WorktreeFiles: []FileSpec{{Path: ".env.local", Mode: FileMaterializeSymlink}},
	}})

	req := materializeCreateRequest(repoPath)
	req.RepositoryID = "" // the production condition
	req.SessionID = "sess-nopath"
	req.TaskDirName = "task-nopath"

	wt, err := mgr.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	info, err := os.Lstat(filepath.Join(wt.Path, ".env.local"))
	if err != nil {
		t.Fatalf(".env.local not materialized via path fallback: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf(".env.local should be a symlink")
	}
}

func TestManagerCreate_NoWorktreeFilesIsNoop(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	if err := os.WriteFile(filepath.Join(repoPath, ".env.local"), []byte("X=1"), 0o644); err != nil {
		t.Fatalf("write .env.local: %v", err)
	}

	// Repository with no configured worktree files (mirrors an existing repo
	// that predates the feature).
	mgr := newMaterializeManager(t, &Repository{ID: "repo-mat"})

	wt, err := mgr.Create(context.Background(), materializeCreateRequest(repoPath))
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(wt.Path, ".env.local")); !os.IsNotExist(err) {
		t.Fatalf("expected no materialized file, got err=%v", err)
	}
}
