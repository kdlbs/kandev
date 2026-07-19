package backendapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	githubsvc "github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/repoclone"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

func TestDetectGitDefaultBranchDetachedHEADReturnsEmpty(t *testing.T) {
	repoPath := t.TempDir()
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(gitDir, "HEAD"),
		[]byte("3a3f2d3b00000000000000000000000000000000\n"),
		0o644,
	); err != nil {
		t.Fatalf("write detached HEAD: %v", err)
	}

	if got := detectGitDefaultBranch(repoPath); got != "" {
		t.Fatalf("detectGitDefaultBranch = %q, want empty", got)
	}
}

func TestResolveReviewBaseBranchRedetectsStoredMasterWhenMainExists(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspace, err := harness.taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
		Name: "Workspace",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	repoPath := t.TempDir()
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "remotes", "origin"), 0o755); err != nil {
		t.Fatalf("mkdir origin refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/x\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(gitDir, "refs", "remotes", "origin", "HEAD"),
		[]byte("ref: refs/remotes/origin/master\n"),
		0o644,
	); err != nil {
		t.Fatalf("write origin HEAD: %v", err)
	}
	for _, branch := range []string{"main", "master"} {
		refPath := filepath.Join(gitDir, "refs", "remotes", "origin", branch)
		if err := os.WriteFile(refPath, []byte("0000000\n"), 0o644); err != nil {
			t.Fatalf("write %s ref: %v", branch, err)
		}
	}
	repo, err := harness.taskSvc.CreateRepository(ctx, &taskservice.CreateRepositoryRequest{
		WorkspaceID:   workspace.ID,
		Name:          "owner/repo",
		SourceType:    "provider",
		LocalPath:     repoPath,
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
		DefaultBranch: "master",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	adapter := &repositoryResolverAdapter{
		taskSvc: harness.taskSvc,
		logger:  newTestLogger(),
	}
	if got := adapter.resolveReviewBaseBranch(ctx, repo, repoPath, ""); got != "main" {
		t.Fatalf("resolveReviewBaseBranch = %q, want main", got)
	}

	stored, err := harness.taskSvc.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.DefaultBranch != "main" {
		t.Fatalf("stored default_branch = %q, want main", stored.DefaultBranch)
	}
}

func TestResolveReviewBaseBranchSkipsNoopPersistForStoredMaster(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspace, err := harness.taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
		Name: "Workspace",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	repoPath := t.TempDir()
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "remotes", "origin"), 0o755); err != nil {
		t.Fatalf("mkdir origin refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/x\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(gitDir, "refs", "remotes", "origin", "HEAD"),
		[]byte("ref: refs/remotes/origin/master\n"),
		0o644,
	); err != nil {
		t.Fatalf("write origin HEAD: %v", err)
	}
	refPath := filepath.Join(gitDir, "refs", "remotes", "origin", "master")
	if err := os.WriteFile(refPath, []byte("0000000\n"), 0o644); err != nil {
		t.Fatalf("write master ref: %v", err)
	}
	repo, err := harness.taskSvc.CreateRepository(ctx, &taskservice.CreateRepositoryRequest{
		WorkspaceID:   workspace.ID,
		Name:          "owner/repo",
		SourceType:    "provider",
		LocalPath:     repoPath,
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
		DefaultBranch: "master",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	adapter := &repositoryResolverAdapter{
		taskSvc: harness.taskSvc,
		logger:  newTestLogger(),
	}
	if got := adapter.resolveReviewBaseBranch(ctx, repo, repoPath, ""); got != "master" {
		t.Fatalf("resolveReviewBaseBranch = %q, want master", got)
	}

	stored, err := harness.taskSvc.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if !stored.UpdatedAt.Equal(repo.UpdatedAt) {
		t.Fatalf("stored updated_at = %v, want unchanged %v", stored.UpdatedAt, repo.UpdatedAt)
	}
}

func TestResolveReviewBaseBranchKeepsStoredMasterOnHeadFallback(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspace, err := harness.taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
		Name: "Workspace",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	repoPath := t.TempDir()
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/x\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	repo, err := harness.taskSvc.CreateRepository(ctx, &taskservice.CreateRepositoryRequest{
		WorkspaceID:   workspace.ID,
		Name:          "owner/repo",
		SourceType:    "provider",
		LocalPath:     repoPath,
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
		DefaultBranch: "master",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	adapter := &repositoryResolverAdapter{
		taskSvc: harness.taskSvc,
		logger:  newTestLogger(),
	}
	if got := adapter.resolveReviewBaseBranch(ctx, repo, repoPath, ""); got != "master" {
		t.Fatalf("resolveReviewBaseBranch = %q, want master", got)
	}

	stored, err := harness.taskSvc.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.DefaultBranch != "master" {
		t.Fatalf("stored default_branch = %q, want master", stored.DefaultBranch)
	}
	if !stored.UpdatedAt.Equal(repo.UpdatedAt) {
		t.Fatalf("stored updated_at = %v, want unchanged %v", stored.UpdatedAt, repo.UpdatedAt)
	}
}

func TestResolveForReviewRedetectsStoredMasterAfterClonePath(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspace, err := harness.taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
		Name: "Workspace",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	basePath := t.TempDir()
	cloner := repoclone.NewCloner(
		repoclone.Config{BasePath: basePath}, repoclone.ProtocolHTTPS, "", newTestLogger(),
	)
	cloner.SetGitCredentialProvider(staticRepoCloneCredential{})
	repoPath, err := cloner.WorkspaceRepoPath(workspace.ID, "github", "owner", "repo")
	if err != nil {
		t.Fatalf("WorkspaceRepoPath: %v", err)
	}
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "remotes", "origin"), 0o755); err != nil {
		t.Fatalf("mkdir origin refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/x\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(gitDir, "refs", "remotes", "origin", "HEAD"),
		[]byte("ref: refs/remotes/origin/master\n"),
		0o644,
	); err != nil {
		t.Fatalf("write origin HEAD: %v", err)
	}
	for _, branch := range []string{"main", "master"} {
		refPath := filepath.Join(gitDir, "refs", "remotes", "origin", branch)
		if err := os.WriteFile(refPath, []byte("0000000\n"), 0o644); err != nil {
			t.Fatalf("write %s ref: %v", branch, err)
		}
	}
	repo, err := harness.taskSvc.CreateRepository(ctx, &taskservice.CreateRepositoryRequest{
		WorkspaceID:   workspace.ID,
		Name:          "owner/repo",
		SourceType:    "provider",
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
		DefaultBranch: "master",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	adapter := &repositoryResolverAdapter{
		cloner:  cloner,
		taskSvc: harness.taskSvc,
		logger:  newTestLogger(),
	}
	repoID, baseBranch, err := adapter.ResolveForReview(ctx, workspace.ID, "github", "owner", "repo", "")
	if err != nil {
		t.Fatalf("ResolveForReview: %v", err)
	}
	if repoID != repo.ID {
		t.Fatalf("repository ID = %q, want %q", repoID, repo.ID)
	}
	if baseBranch != "main" {
		t.Fatalf("base branch = %q, want main", baseBranch)
	}

	stored, err := harness.taskSvc.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.LocalPath != repoPath {
		t.Fatalf("stored local_path = %q, want %q", stored.LocalPath, repoPath)
	}
	if stored.DefaultBranch != "main" {
		t.Fatalf("stored default_branch = %q, want main", stored.DefaultBranch)
	}
}

type staticRepoCloneCredential struct{}

func (staticRepoCloneCredential) ResolveGitCredential(
	context.Context,
	string,
	string,
	string,
	string,
) (string, string, error) {
	return "x-access-token", "test-token", nil
}

func TestGetSessionModel_Caching(t *testing.T) {
	// We can't easily instantiate a full taskservice.Service here without a DB,
	// so we test the caching mechanism directly on the adapter struct.
	adapter := &messageCreatorAdapter{
		svc:    nil, // Will cause getSessionModel to return "" on cache miss (nil svc panics)
		logger: newTestLogger(),
	}

	// Pre-populate the cache to avoid calling the nil svc
	adapter.sessionModelMu.Lock()
	adapter.sessionModelCache = map[string]string{
		"session-1": "claude-sonnet-4",
		"session-2": "gpt-4",
	}
	adapter.sessionModelMu.Unlock()

	// Test cache hit
	model := adapter.getSessionModel(context.Background(), "session-1")
	if model != "claude-sonnet-4" {
		t.Errorf("expected 'claude-sonnet-4', got %q", model)
	}

	model = adapter.getSessionModel(context.Background(), "session-2")
	if model != "gpt-4" {
		t.Errorf("expected 'gpt-4', got %q", model)
	}

	// Test cache miss for unknown session returns ""
	// (svc is nil, so DB lookup would fail gracefully)
	// We need a non-nil svc to avoid panic - use a minimal mock approach
	// Instead, verify the cache was populated for existing entries
	adapter.sessionModelMu.RLock()
	if len(adapter.sessionModelCache) != 2 {
		t.Errorf("expected 2 cached entries, got %d", len(adapter.sessionModelCache))
	}
	adapter.sessionModelMu.RUnlock()
}

func TestGetSessionModel_ConcurrentAccess(t *testing.T) {
	adapter := &messageCreatorAdapter{
		svc:    nil,
		logger: newTestLogger(),
	}

	// Pre-populate cache
	adapter.sessionModelMu.Lock()
	adapter.sessionModelCache = map[string]string{
		"session-1": "claude-sonnet-4",
	}
	adapter.sessionModelMu.Unlock()

	// Concurrent reads should not race
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			model := adapter.getSessionModel(context.Background(), "session-1")
			if model != "claude-sonnet-4" {
				t.Errorf("expected 'claude-sonnet-4', got %q", model)
			}
		}()
	}
	wg.Wait()
}

func TestGetSessionModel_LazyInit(t *testing.T) {
	// Verify that the cache map is lazily initialized (nil initially)
	adapter := &messageCreatorAdapter{
		svc:    nil,
		logger: newTestLogger(),
	}

	// sessionModelCache should be nil initially
	adapter.sessionModelMu.RLock()
	if adapter.sessionModelCache != nil {
		t.Error("expected sessionModelCache to be nil initially")
	}
	adapter.sessionModelMu.RUnlock()
}

// Verify the adapter compiles with the taskservice.Service field
func TestMessageCreatorAdapter_StructFields(t *testing.T) {
	adapter := &messageCreatorAdapter{
		svc:    (*taskservice.Service)(nil),
		logger: newTestLogger(),
	}
	if adapter.svc != nil {
		t.Error("expected nil svc")
	}
}

func TestWrapGitHubTaskIssueStoreError(t *testing.T) {
	taskErr := fmt.Errorf("load task: %w", taskrepo.ErrTaskNotFound)
	wrapped := wrapGitHubTaskIssueStoreError(taskErr)
	if !errors.Is(wrapped, githubsvc.ErrTaskNotFound) {
		t.Fatalf("wrapped error should match github ErrTaskNotFound: %v", wrapped)
	}
	if !errors.Is(wrapped, taskrepo.ErrTaskNotFound) {
		t.Fatalf("wrapped error should preserve task repo ErrTaskNotFound: %v", wrapped)
	}

	otherErr := errors.New("database unavailable")
	if got := wrapGitHubTaskIssueStoreError(otherErr); got != otherErr {
		t.Fatalf("non-not-found error changed: got %v, want %v", got, otherErr)
	}
}

// jiraSecretAdapter Set/Exists branching is tested in
// internal/integrations/secretadapter/secretadapter_test.go now that the
// upsert helper lives there.
