package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func commitTestHarnessGeneration(
	t *testing.T,
	repo *Repository,
	sessionID, incarnationID, workspace string,
	generation int64,
) {
	t.Helper()
	now := time.Now().UTC().Add(time.Duration(generation) * time.Second)
	committed, err := repo.CommitHarnessSessionGeneration(context.Background(), &models.HarnessSessionGeneration{
		SessionID:        sessionID,
		IncarnationID:    incarnationID,
		Generation:       generation,
		NativeSessionID:  "native-" + sessionID,
		CreationReason:   "initial",
		CurrentWorkspace: workspace,
		CreatedAt:        now,
		CommittedAt:      now,
	}, generation-1)
	if err != nil {
		t.Fatalf("CommitHarnessSessionGeneration(%s, %d): %v", sessionID, generation, err)
	}
	if !committed {
		t.Fatalf("CommitHarnessSessionGeneration(%s, %d) was rejected", sessionID, generation)
	}
}

func createAgentWorkspaceFixture(
	t *testing.T,
	repo *Repository,
	taskID, sessionID, incarnationID, environmentRoot, sessionWorkspace string,
) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Agent workspace"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:            "env-" + taskID,
		TaskID:        taskID,
		ExecutorType:  string(models.ExecutorTypeWorktree),
		Status:        models.TaskEnvironmentStatusReady,
		WorkspacePath: environmentRoot,
		Repos: []*models.TaskEnvironmentRepo{{
			RepositoryID: "repo-" + taskID,
			WorktreeID:   "worktree-" + taskID,
			WorktreePath: environmentRoot + "/master",
		}},
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                 sessionID,
		TaskID:             taskID,
		QueueIncarnationID: incarnationID,
		TaskEnvironmentID:  "env-" + taskID,
		WorkspacePath:      sessionWorkspace,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
}

func TestTaskSessionAgentWorkspaceProjection(t *testing.T) {
	t.Run("distinct effective primary and native roots after promotion", func(t *testing.T) {
		repo := newRepoForSessionTests(t)
		ctx := context.Background()
		createAgentWorkspaceFixture(t, repo, "task-promoted", "session-promoted", "inc-promoted", "/task-promoted", "/task-promoted/master")
		commitTestHarnessGeneration(t, repo, "session-promoted", "inc-promoted", "/task-promoted/master", 1)

		// An ordinary session update overwrites the mutable column with the
		// promoted environment root; the native projection must survive it.
		if _, err := repo.db.ExecContext(ctx,
			repo.db.Rebind(`UPDATE task_sessions SET workspace_path = ? WHERE id = ?`),
			"/task-promoted", "session-promoted"); err != nil {
			t.Fatalf("update session workspace: %v", err)
		}

		got, err := repo.GetTaskSession(ctx, "session-promoted")
		if err != nil {
			t.Fatalf("GetTaskSession: %v", err)
		}
		if got.WorkspacePath != "/task-promoted" {
			t.Fatalf("WorkspacePath = %q, want %q", got.WorkspacePath, "/task-promoted")
		}
		if got.AgentWorkspacePath != "/task-promoted/master" {
			t.Fatalf("AgentWorkspacePath = %q, want %q", got.AgentWorkspacePath, "/task-promoted/master")
		}
		if len(got.Worktrees) != 1 || got.Worktrees[0].WorktreePath != "/task-promoted/master" {
			t.Fatalf("primary worktree = %+v, want /task-promoted/master", got.Worktrees)
		}
	})

	t.Run("parent-root launch keeps the parent as the native workspace", func(t *testing.T) {
		repo := newRepoForSessionTests(t)
		ctx := context.Background()
		createAgentWorkspaceFixture(t, repo, "task-parent", "session-parent", "inc-parent", "/task-parent", "/task-parent")
		commitTestHarnessGeneration(t, repo, "session-parent", "inc-parent", "/task-parent", 1)

		got, err := repo.GetTaskSession(ctx, "session-parent")
		if err != nil {
			t.Fatalf("GetTaskSession: %v", err)
		}
		if got.WorkspacePath != "/task-parent" || got.AgentWorkspacePath != "/task-parent" {
			t.Fatalf("roots = %q / %q, want both /task-parent", got.WorkspacePath, got.AgentWorkspacePath)
		}
	})

	t.Run("current incarnation outranks a predecessor generation", func(t *testing.T) {
		repo := newRepoForSessionTests(t)
		ctx := context.Background()
		createAgentWorkspaceFixture(t, repo, "task-incarnation", "session-incarnation", "inc-one", "/task-incarnation", "/task-incarnation/master")
		commitTestHarnessGeneration(t, repo, "session-incarnation", "inc-one", "/task-incarnation/master", 1)
		commitTestHarnessGeneration(t, repo, "session-incarnation", "inc-two", "/task-incarnation/next", 1)

		if _, err := repo.db.ExecContext(ctx,
			repo.db.Rebind(`UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?`),
			"inc-two", "session-incarnation"); err != nil {
			t.Fatalf("update incarnation: %v", err)
		}

		got, err := repo.GetTaskSession(ctx, "session-incarnation")
		if err != nil {
			t.Fatalf("GetTaskSession: %v", err)
		}
		if got.AgentWorkspacePath != "/task-incarnation/next" {
			t.Fatalf("AgentWorkspacePath = %q, want current incarnation value", got.AgentWorkspacePath)
		}
	})

	t.Run("latest committed generation wins within one incarnation", func(t *testing.T) {
		repo := newRepoForSessionTests(t)
		ctx := context.Background()
		createAgentWorkspaceFixture(t, repo, "task-generation", "session-generation", "inc-generation", "/task-generation", "/task-generation/master")
		commitTestHarnessGeneration(t, repo, "session-generation", "inc-generation", "/task-generation/master", 1)
		commitTestHarnessGeneration(t, repo, "session-generation", "inc-generation", "/task-generation/secondary", 2)

		got, err := repo.GetTaskSession(ctx, "session-generation")
		if err != nil {
			t.Fatalf("GetTaskSession: %v", err)
		}
		if got.AgentWorkspacePath != "/task-generation/secondary" {
			t.Fatalf("AgentWorkspacePath = %q, want newest generation", got.AgentWorkspacePath)
		}
	})

	t.Run("legacy incarnation falls back to the session identity", func(t *testing.T) {
		repo := newRepoForSessionTests(t)
		ctx := context.Background()
		createAgentWorkspaceFixture(t, repo, "task-legacy", "session-legacy", "inc-placeholder", "/task-legacy", "/task-legacy/master")
		// Legacy rows predate queue incarnations; the session identity is the
		// documented fallback when the column is empty.
		if _, err := repo.db.ExecContext(ctx,
			repo.db.Rebind(`UPDATE task_sessions SET queue_incarnation_id = '' WHERE id = ?`),
			"session-legacy"); err != nil {
			t.Fatalf("clear incarnation: %v", err)
		}
		commitTestHarnessGeneration(t, repo, "session-legacy", "session-legacy", "/task-legacy/master", 1)

		got, err := repo.GetTaskSession(ctx, "session-legacy")
		if err != nil {
			t.Fatalf("GetTaskSession: %v", err)
		}
		if got.AgentWorkspacePath != "/task-legacy/master" {
			t.Fatalf("AgentWorkspacePath = %q, want legacy fallback value", got.AgentWorkspacePath)
		}
	})

	t.Run("missing generation leaves the projection unknown", func(t *testing.T) {
		repo := newRepoForSessionTests(t)
		ctx := context.Background()
		createAgentWorkspaceFixture(t, repo, "task-unknown", "session-unknown", "inc-unknown", "/task-unknown", "/task-unknown")

		got, err := repo.GetTaskSession(ctx, "session-unknown")
		if err != nil {
			t.Fatalf("GetTaskSession: %v", err)
		}
		if got.AgentWorkspacePath != "" {
			t.Fatalf("AgentWorkspacePath = %q, want empty for missing generation", got.AgentWorkspacePath)
		}
		observations, err := repo.ListTaskSessionSummaryObservations(ctx, "task-unknown")
		if err != nil {
			t.Fatalf("ListTaskSessionSummaryObservations: %v", err)
		}
		if len(observations) != 1 || observations[0].AgentWorkspacePath != "" {
			t.Fatalf("summary observation = %+v, want empty native workspace", observations)
		}
	})

	t.Run("summary projection carries the native workspace", func(t *testing.T) {
		repo := newRepoForSessionTests(t)
		ctx := context.Background()
		createAgentWorkspaceFixture(t, repo, "task-summary", "session-summary", "inc-summary", "/task-summary", "/task-summary/master")
		commitTestHarnessGeneration(t, repo, "session-summary", "inc-summary", "/task-summary/master", 1)

		observations, err := repo.ListTaskSessionSummaryObservations(ctx, "task-summary")
		if err != nil {
			t.Fatalf("ListTaskSessionSummaryObservations: %v", err)
		}
		if len(observations) != 1 || observations[0].AgentWorkspacePath != "/task-summary/master" {
			t.Fatalf("summary observation = %+v, want native workspace", observations)
		}
	})
}
