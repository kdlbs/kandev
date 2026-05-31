package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/worktree"
)

// TestWorktreePreparer_SetupScriptFailure_KeepsCreateWorktreeStepCompleted
// guards the step-attribution edge case introduced alongside the early
// completion of the "Create worktree" step.
//
// The per-repo setup script runs inside worktree.Manager.Create() AFTER the
// worktree directory is created (git worktree add succeeded) and the
// OnWorktreeCreated callback has already completed the "Create worktree" step.
// A setup-script failure is non-fatal: Create() keeps the worktree, records a
// warning, and returns no error, so preparation still succeeds and the agent
// launches. The failure belongs to the setup script (which streams its own
// step), not to worktree creation — so "Create worktree" stays completed and
// carries the setup-script warning rather than being re-marked as failed.
func TestWorktreePreparer_SetupScriptFailure_KeepsCreateWorktreeStepCompleted(t *testing.T) {
	repo := initBareGitRepo(t, "single")

	repos := map[string]*worktree.Repository{
		"repo-single": {ID: "repo-single", SetupScript: "echo hi"},
	}
	preparer, _, handler := newPreparerWithScriptHandler(t, repos)
	handler.scriptError = errors.New("setup script boom")

	req := &EnvPrepareRequest{
		TaskID:          "task-setup-fail",
		SessionID:       "sess-setup-fail",
		TaskTitle:       "Setup Fail",
		ExecutorType:    executor.NameStandalone,
		TaskDirName:     "setup-fail_fff",
		UseWorktree:     true,
		RepositoryID:    "repo-single",
		RepositoryPath:  repo,
		RepoName:        "single",
		BaseBranch:      "main",
		RepoSetupScript: "echo hi",
	}

	res, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("prepare returned hard error: %v", err)
	}
	// A failed setup script is non-fatal: preparation still succeeds so the
	// agent launches (and Resume / Start-fresh keep working).
	if !res.Success {
		t.Fatalf("expected prepare to succeed (setup-script failures are non-fatal)")
	}

	var createStep *PrepareStep
	for i := range res.Steps {
		if res.Steps[i].Name == "Create worktree" {
			createStep = &res.Steps[i]
		}
	}
	if createStep == nil {
		t.Fatal(`no "Create worktree" step found in prepare result`)
	}
	if createStep.Status != PrepareStepCompleted {
		t.Errorf(`"Create worktree" status = %q, want %q — a setup-script failure must not be attributed to worktree creation`,
			createStep.Status, PrepareStepCompleted)
	}
	if createStep.Error != "" {
		t.Errorf(`"Create worktree" unexpectedly carries error %q`, createStep.Error)
	}
	// The non-fatal failure surfaces as a warning on the step instead.
	if createStep.Warning == "" {
		t.Errorf(`"Create worktree" should carry the setup-script warning when the script fails`)
	}
}
