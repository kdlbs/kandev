package worktree

import (
	"context"
	"errors"
	"os"
	"testing"
)

// fakeScriptHandler simulates the script message handler. ExecuteSetupScript
// returns setupErr so tests can drive the setup-script failure path without a
// real task service / shell.
type fakeScriptHandler struct {
	setupErr  error
	setupRuns int
}

func (f *fakeScriptHandler) ExecuteSetupScript(_ context.Context, _ ScriptExecutionRequest) error {
	f.setupRuns++
	return f.setupErr
}

func (f *fakeScriptHandler) ExecuteCleanupScript(_ context.Context, _ ScriptExecutionRequest) error {
	return nil
}

func newManagerForSetupTest(t *testing.T, provider RepositoryProvider, handler ScriptMessageHandler) *Manager {
	t.Helper()
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	if provider != nil {
		mgr.SetRepositoryProvider(provider)
	}
	if handler != nil {
		mgr.SetScriptMessageHandler(handler)
	}
	return mgr
}

// TestManagerCreate_SetupScriptFailure_NonFatal covers the regression where a
// failing repository setup script aborted environment preparation (and the
// launch) instead of being surfaced as a non-fatal warning. The worktree must
// survive the failure and the warning must be recorded on the worktree so the
// UI can show it without blocking the task.
func TestManagerCreate_SetupScriptFailure_NonFatal(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	provider := &fakeRepoProvider{
		repo: &Repository{ID: "repo-setupfail", SetupScript: "make install"},
	}
	handler := &fakeScriptHandler{setupErr: errors.New("script exited with code 2")}
	mgr := newManagerForSetupTest(t, provider, handler)

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-setupfail",
		SessionID:      "session-setupfail",
		TaskTitle:      "Setup script fails",
		RepositoryID:   "repo-setupfail",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		TaskDirName:    "task-setupfail",
		RepoName:       "repo-setupfail",
	})
	if err != nil {
		t.Fatalf("Create() should not fail when the setup script fails, got: %v", err)
	}
	if wt == nil {
		t.Fatal("expected non-nil worktree after non-fatal setup failure")
	}
	if handler.setupRuns != 1 {
		t.Fatalf("setup script run count = %d, want 1", handler.setupRuns)
	}
	// The worktree directory must survive — the agent needs it to work in.
	if _, statErr := os.Stat(wt.Path); statErr != nil {
		t.Fatalf("expected worktree dir to survive setup failure, stat err = %v", statErr)
	}
	if wt.SetupScriptWarning == "" {
		t.Fatal("expected SetupScriptWarning to be set when the setup script fails")
	}
}

// TestManagerCreate_SetupScriptSuccess_NoWarning guards against the warning
// being set on the happy path.
func TestManagerCreate_SetupScriptSuccess_NoWarning(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	provider := &fakeRepoProvider{
		repo: &Repository{ID: "repo-setupok", SetupScript: "make install"},
	}
	handler := &fakeScriptHandler{setupErr: nil}
	mgr := newManagerForSetupTest(t, provider, handler)

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-setupok",
		SessionID:      "session-setupok",
		TaskTitle:      "Setup script succeeds",
		RepositoryID:   "repo-setupok",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		TaskDirName:    "task-setupok",
		RepoName:       "repo-setupok",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if wt.SetupScriptWarning != "" {
		t.Fatalf("expected no setup-script warning on success, got %q", wt.SetupScriptWarning)
	}
}
