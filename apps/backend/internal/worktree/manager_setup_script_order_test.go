package worktree

import (
	"context"
	"testing"
)

// orderRecordingScriptHandler records when the per-repo setup script runs so a
// test can assert it relative to the OnWorktreeCreated callback.
type orderRecordingScriptHandler struct {
	order *[]string
}

func (h *orderRecordingScriptHandler) ExecuteSetupScript(_ context.Context, _ ScriptExecutionRequest) error {
	*h.order = append(*h.order, "script")
	return nil
}

func (h *orderRecordingScriptHandler) ExecuteCleanupScript(_ context.Context, _ ScriptExecutionRequest) error {
	return nil
}

// TestManagerCreate_OnWorktreeCreatedFiresBeforeSetupScript guards the prepare
// UI ordering. The worktree-ready signal (used by the env preparer to complete
// the "Create worktree" step) must fire AFTER the worktree directory exists but
// BEFORE the per-repo setup script runs — otherwise the "Create worktree" and
// "Run repository setup script" steps render as overlapping spinners instead of
// a sequential timeline.
func TestManagerCreate_OnWorktreeCreatedFiresBeforeSetupScript(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)

	var order []string
	provider := &fakeRepoProvider{
		repo: &Repository{ID: "repo-order", SetupScript: "echo hi"},
	}
	mgr := newManagerForCopyTest(t, provider)
	mgr.SetScriptMessageHandler(&orderRecordingScriptHandler{order: &order})

	req := createReqForCopyTest(repoPath, "order")
	req.OnWorktreeCreated = func(_ *Worktree) {
		order = append(order, "callback")
	}

	if _, err := mgr.Create(context.Background(), req); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	want := []string{"callback", "script"}
	if len(order) != len(want) || order[0] != want[0] || order[1] != want[1] {
		t.Fatalf("execution order = %v, want %v", order, want)
	}
}

// TestManagerCreate_OnWorktreeCreatedReceivesFallbackWarning verifies the
// worktree handed to the callback already carries the base-branch fallback
// warning, so the "Create worktree" step can surface it when completed early.
func TestManagerCreate_OnWorktreeCreatedReceivesFallbackWarning(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)

	mgr := newManagerForCopyTest(t, nil)

	req := createReqForCopyTest(repoPath, "fallback")
	req.BaseBranch = "does-not-exist"
	req.FallbackBaseBranch = "main"

	var warningAtCallback string
	req.OnWorktreeCreated = func(wt *Worktree) {
		warningAtCallback = wt.BaseBranchFallbackWarning
	}

	if _, err := mgr.Create(context.Background(), req); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	if warningAtCallback == "" {
		t.Fatalf("expected fallback warning to be set on worktree at callback time, got empty")
	}
}
