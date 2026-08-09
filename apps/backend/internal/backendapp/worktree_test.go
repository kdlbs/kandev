package backendapp

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

func TestBootMessageAdapterResumedMessageDoesNotLeaveActiveTurn(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()

	if err := harness.taskRepo.CreateWorkspace(ctx, &models.Workspace{
		ID:   "workspace-1",
		Name: "Workspace",
	}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := harness.taskRepo.CreateTask(ctx, &models.Task{
		ID:          "task-1",
		WorkspaceID: "workspace-1",
		Title:       "Task",
		Priority:    "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := harness.taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	message, err := (&bootMsgAdapter{svc: harness.taskSvc}).CreateMessage(
		ctx,
		&lifecycle.BootMessageRequest{
			TaskSessionID: "session-1",
			TaskID:        "task-1",
			AuthorType:    "agent",
			Type:          "script_execution",
			Metadata: map[string]interface{}{
				"script_type": "agent_boot",
				"is_resuming": true,
			},
		},
	)
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if message == nil {
		t.Fatal("CreateMessage returned nil")
	}

	activeTurn, err := harness.taskSvc.GetActiveTurn(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetActiveTurn: %v", err)
	}
	if activeTurn != nil {
		t.Fatalf("active turn = %q, want none", activeTurn.ID)
	}

	turn, err := harness.taskSvc.GetTurn(ctx, message.TurnID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if turn.CompletedAt == nil {
		t.Fatalf("resume message turn %q is not completed", turn.ID)
	}
}

// TestDetectBranchRemote_ReturnsConfiguredUpstream covers the happy path: a
// branch with an explicit `branch.<name>.remote` config returns that remote
// (covers fork-workflow repos whose primary remote is named "upstream",
// "github", etc., where hard-coding "origin" would push to the wrong place).
func TestDetectBranchRemote_ReturnsConfiguredUpstream(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	mustGit(t, dir, "init", "--quiet")
	mustGit(t, dir, "commit", "--allow-empty", "-m", "init")
	mustGit(t, dir, "checkout", "-b", "feature")
	// Mimic what `git push --set-upstream <remote> <branch>` would write.
	mustGit(t, dir, "config", "branch.feature.remote", "upstream")

	if got := detectBranchRemote(context.Background(), dir, "feature"); got != "upstream" {
		t.Errorf("got %q, want upstream", got)
	}
}

func TestDetectBranchRemote_NoUpstreamFallsBackToOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	mustGit(t, dir, "init", "--quiet")
	mustGit(t, dir, "commit", "--allow-empty", "-m", "init")
	mustGit(t, dir, "checkout", "-b", "feature")
	// No branch.feature.remote config — git config returns non-zero.

	if got := detectBranchRemote(context.Background(), dir, "feature"); got != defaultGitRemote {
		t.Errorf("got %q, want %s", got, defaultGitRemote)
	}
}

func TestDetectBranchRemote_NonGitDirFallsBackToOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	// `git config` in a non-git dir errors out, so detectBranchRemote
	// should fall back to the default remote rather than propagate the error.
	if got := detectBranchRemote(context.Background(), t.TempDir(), "feature"); got != defaultGitRemote {
		t.Errorf("got %q, want %s", got, defaultGitRemote)
	}
}

// mustGit runs `git -C dir <args...>` and fails the test on non-zero exit.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", full...)
	// Start from the parent environment so git keeps HOME/PATH/etc., then:
	//   - disable system config (/etc/gitconfig) to prevent host policy from
	//     interfering with the test
	//   - set an identity inline so `git commit` works on CI runners that
	//     have no user.email/user.name configured. Both AUTHOR and COMMITTER
	//     are needed — git fails fast if either is missing.
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
