package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestWorktreePreparer_MultiRepo_RollbackKeepsDirtyRetainedCheckout(t *testing.T) {
	repoA := initBareGitRepo(t, "retained-good")
	repoB := initBareGitRepo(t, "retained-bad")
	preparer, mgr, store := newPreparerForTestWithStore(t)

	const (
		taskID  = "task-retained-rollback"
		taskDir = "retained-rollback_123"
		repoID  = "repo-retained-good"
		branch  = "feature/retained"
	)
	retained, err := mgr.Create(context.Background(), worktree.CreateRequest{
		TaskID:         taskID,
		SessionID:      "session-before-prepare",
		RepositoryID:   repoID,
		RepositoryPath: repoA,
		BaseBranch:     "main",
		CheckoutBranch: branch,
		TaskDirName:    taskDir,
		RepoName:       "good",
	})
	if err != nil {
		t.Fatalf("create retained checkout: %v", err)
	}
	if err := store.DeleteWorktree(context.Background(), retained.ID); err != nil {
		t.Fatalf("remove retained checkout inventory row: %v", err)
	}

	readme := filepath.Join(retained.Path, "README.md")
	if err := os.WriteFile(readme, []byte("staged change\n"), 0o644); err != nil {
		t.Fatalf("write staged change: %v", err)
	}
	runGitForPreparerTest(t, retained.Path, "add", "README.md")
	if err := os.WriteFile(readme, []byte("unstaged change\n"), 0o644); err != nil {
		t.Fatalf("write unstaged change: %v", err)
	}
	untracked := filepath.Join(retained.Path, "notes.txt")
	if err := os.WriteFile(untracked, []byte("untracked work\n"), 0o644); err != nil {
		t.Fatalf("write untracked change: %v", err)
	}

	result, err := preparer.Prepare(context.Background(), &EnvPrepareRequest{
		TaskID:       taskID,
		SessionID:    "session-after-prepare",
		ExecutorType: executor.NameStandalone,
		TaskDirName:  taskDir,
		Repositories: []RepoPrepareSpec{
			{RepositoryID: repoID, RepositoryPath: repoA, RepoName: "good", BaseBranch: "main", CheckoutBranch: branch},
			{RepositoryID: "repo-retained-bad", RepositoryPath: repoB, RepoName: "bad", BaseBranch: "missing-branch"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("prepare returned hard error (should report via result): %v", err)
	}
	if result.Success {
		t.Fatal("expected failure when the second repository has no requested base branch")
	}

	if _, err := os.Stat(retained.Path); err != nil {
		t.Fatalf("retained checkout path was removed: %v", err)
	}
	if got, err := os.ReadFile(readme); err != nil || string(got) != "unstaged change\n" {
		t.Fatalf("retained tracked file = %q, %v; want unstaged change", got, err)
	}
	if got, err := os.ReadFile(untracked); err != nil || string(got) != "untracked work\n" {
		t.Fatalf("retained untracked file = %q, %v; want untracked work", got, err)
	}
	status := runGitForPreparerTest(t, retained.Path, "status", "--porcelain")
	if !strings.Contains(status, "MM README.md") || !strings.Contains(status, "?? notes.txt") {
		t.Fatalf("retained checkout status = %q, want staged, unstaged, and untracked changes", status)
	}

	rows, err := store.GetWorktreesByTaskID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("list retained checkout registration: %v", err)
	}
	if len(rows) != 1 || rows[0].ID == retained.ID || rows[0].Status != worktree.StatusActive || rows[0].Path != retained.Path {
		t.Fatalf("retained checkout registration = %+v, want one active row for the same path and new ID", rows)
	}
}
