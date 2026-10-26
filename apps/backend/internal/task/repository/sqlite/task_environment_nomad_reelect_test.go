package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// addExecutorsRunningRow stores a retained executors_running row for a session.
func addExecutorsRunningRow(t *testing.T, repo *Repository, sessionID, taskID, runtime, metadata string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO executors_running (id, session_id, task_id, executor_id, runtime, status, agent_execution_id, metadata, created_at, updated_at)
		VALUES (?, ?, ?, 'executor-reelect', ?, 'starting', ?, ?, ?, ?)
	`), "running-"+sessionID, sessionID, taskID, runtime, "execution-"+sessionID, metadata, now, now); err != nil {
		t.Fatalf("insert executors_running for %s: %v", sessionID, err)
	}
}

func environmentStatus(t *testing.T, repo *Repository, environmentID string) (models.TaskEnvironmentStatus, string) {
	t.Helper()
	env, err := repo.GetTaskEnvironment(context.Background(), environmentID)
	if err != nil {
		t.Fatal(err)
	}
	return env.Status, env.MaterializationSessionID
}

func TestReelectNomad_CreatingTakeOver(t *testing.T) {
	for name, metadata := range map[string]string{"valid metadata": `{"unrelated":true}`, "malformed metadata": `not json`, "empty metadata": ``} {
		t.Run(name, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			seedMaterializationClaimTask(t, repo, "task-reelect-take")
			owner := seedCreatingOwner(t, repo, "task-reelect-take", "session-reelect-owner", models.TaskSessionStateCreated)
			ageTaskEnvironment(t, repo, owner.TaskEnvironmentID, 24*time.Hour)
			addExecutorsRunningRow(t, repo, owner.ID, owner.TaskID, "nomad", metadata)

			_, err := bindRetry(repo, "task-reelect-take", "session-reelect-retry")
			if !errors.Is(err, models.ErrWorkspacePreparing) {
				t.Fatalf("bind against retained Nomad row error = %v, want workspace preparing", err)
			}
			assertCreatingOwner(t, repo, owner.TaskEnvironmentID, owner.ID)
		})
	}
}

func TestReelectNomad_RowOfSiblingSessionBoundToEnvironment(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedMaterializationClaimTask(t, repo, "task-reelect-sibling")
	owner := seedCreatingOwner(t, repo, "task-reelect-sibling", "session-sibling-owner", models.TaskSessionStateCreated)
	ageTaskEnvironment(t, repo, owner.TaskEnvironmentID, 24*time.Hour)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-sibling-bound", TaskID: owner.TaskID, TaskEnvironmentID: owner.TaskEnvironmentID}); err != nil {
		t.Fatal(err)
	}
	addExecutorsRunningRow(t, repo, "session-sibling-bound", owner.TaskID, "nomad", `{}`)

	if _, err := bindRetry(repo, owner.TaskID, "session-sibling-retry"); !errors.Is(err, models.ErrWorkspacePreparing) {
		t.Fatalf("bind error = %v, want workspace preparing", err)
	}
	assertCreatingOwner(t, repo, owner.TaskEnvironmentID, owner.ID)
}

func TestReelectNomad_FailAbandoned(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedMaterializationClaimTask(t, repo, "task-reelect-fail")
	owner := seedCreatingOwner(t, repo, "task-reelect-fail", "session-reelect-fail-owner", models.TaskSessionStateFailed)
	addWorktreeEvidence(t, repo, owner.TaskEnvironmentID)
	addExecutorsRunningRow(t, repo, owner.ID, owner.TaskID, "nomad", `{}`)

	_, err := bindRetry(repo, "task-reelect-fail", "session-reelect-fail-retry")
	if !errors.Is(err, models.ErrWorkspacePreparing) {
		t.Fatalf("bind error = %v, want workspace preparing (not reuse unsafe)", err)
	}
	if status, owned := environmentStatus(t, repo, owner.TaskEnvironmentID); status != models.TaskEnvironmentStatusCreating || owned != owner.ID {
		t.Fatalf("environment = %q owned by %q, want unchanged creating", status, owned)
	}
}

func TestReelectNomad_MissingOrTerminalOwnerWithRetainedRow(t *testing.T) {
	t.Run("terminal owner", func(t *testing.T) {
		for _, state := range []models.TaskSessionState{models.TaskSessionStateCancelled, models.TaskSessionStateFailed, models.TaskSessionStateCompleted} {
			repo := newRepoForEntityTests(t)
			seedMaterializationClaimTask(t, repo, "task-reelect-terminal")
			owner := seedCreatingOwner(t, repo, "task-reelect-terminal", "session-terminal-owner", state)
			addExecutorsRunningRow(t, repo, owner.ID, owner.TaskID, "nomad", `{}`)

			if _, err := bindRetry(repo, owner.TaskID, "session-terminal-retry"); !errors.Is(err, models.ErrWorkspacePreparing) {
				t.Fatalf("%s owner: bind error = %v, want workspace preparing", state, err)
			}
			if status, owned := environmentStatus(t, repo, owner.TaskEnvironmentID); status != models.TaskEnvironmentStatusCreating || owned != owner.ID {
				t.Fatalf("%s owner: environment = %q owned by %q, want unchanged creating", state, status, owned)
			}
		}
	})
	t.Run("missing owner", func(t *testing.T) {
		repo := newRepoForEntityTests(t)
		ctx := context.Background()
		seedMaterializationClaimTask(t, repo, "task-reelect-missing")
		owner := seedCreatingOwner(t, repo, "task-reelect-missing", "session-missing-owner", models.TaskSessionStateCreated)
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-missing-bound", TaskID: owner.TaskID, TaskEnvironmentID: owner.TaskEnvironmentID}); err != nil {
			t.Fatal(err)
		}
		addExecutorsRunningRow(t, repo, "session-missing-bound", owner.TaskID, "nomad", `{}`)
		if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_environments SET materialization_session_id = 'session-never-existed' WHERE id = ?`), owner.TaskEnvironmentID); err != nil {
			t.Fatal(err)
		}

		if _, err := bindRetry(repo, owner.TaskID, "session-missing-retry"); !errors.Is(err, models.ErrWorkspacePreparing) {
			t.Fatalf("bind error = %v, want workspace preparing", err)
		}
		if status, owned := environmentStatus(t, repo, owner.TaskEnvironmentID); status != models.TaskEnvironmentStatusCreating || owned != "session-never-existed" {
			t.Fatalf("environment = %q owned by %q, want unchanged creating", status, owned)
		}
	})
}

func TestReelectNomad_FailedReelectCallers(t *testing.T) {
	seed := func(t *testing.T, taskID string, withNomadRow bool) (*Repository, string) {
		repo := newRepoForEntityTests(t)
		seedMaterializationClaimTask(t, repo, taskID)
		environmentID := seedFailedEnvironment(t, repo, taskID, "session-"+taskID+"-owner")
		if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{ID: "session-" + taskID + "-bound", TaskID: taskID, TaskEnvironmentID: environmentID}); err != nil {
			t.Fatal(err)
		}
		if withNomadRow {
			addExecutorsRunningRow(t, repo, "session-"+taskID+"-bound", taskID, "nomad", `{}`)
		}
		return repo, environmentID
	}
	requireRetained := func(t *testing.T, repo *Repository, environmentID string) {
		t.Helper()
		if status, owned := environmentStatus(t, repo, environmentID); status != models.TaskEnvironmentStatusFailed || owned != "" {
			t.Fatalf("environment = %q owned by %q, want unchanged failed", status, owned)
		}
	}

	t.Run("binding", func(t *testing.T) {
		repo, environmentID := seed(t, "task-failed-binding", true)
		if _, err := bindRetry(repo, "task-failed-binding", "session-failed-binding-retry"); !errors.Is(err, models.ErrTaskEnvironmentResetRequired) {
			t.Fatalf("bind error = %v, want reset required", err)
		}
		requireRetained(t, repo, environmentID)

		repo, environmentID = seed(t, "task-failed-binding-control", false)
		if _, err := bindRetry(repo, "task-failed-binding-control", "session-failed-binding-control-retry"); err != nil {
			t.Fatalf("control bind without a Nomad row = %v, want re-election", err)
		}
		assertCreatingOwner(t, repo, environmentID, "session-failed-binding-control-retry")
	})
	t.Run("shared group binding", func(t *testing.T) {
		for _, withNomadRow := range []bool{true, false} {
			repo := newRepoForEntityTests(t)
			ctx := context.Background()
			seedWorkspace(t, repo, "workspace-group-nomad")
			for _, taskID := range []string{"task-group-nomad-a", "task-group-nomad-b"} {
				if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "workspace-group-nomad", Title: taskID}); err != nil {
					t.Fatal(err)
				}
			}
			seedSharedWorkspaceGroup(t, repo, "group-nomad", "task-group-nomad-a", "task-group-nomad-b")
			owner := &models.TaskSession{ID: "session-group-nomad-owner", TaskID: "task-group-nomad-a"}
			if err := repo.CreateTaskSessionWithSharedGroupWorkspaceBinding(ctx, owner, &models.TaskEnvironment{
				TaskID: owner.TaskID, ExecutorType: string(models.ExecutorTypeLocal),
			}, "group-nomad"); err != nil {
				t.Fatal(err)
			}
			if changed, err := repo.FailTaskEnvironmentMaterialization(ctx, owner.ID); err != nil || !changed {
				t.Fatalf("fail group claim = %v, %v", changed, err)
			}
			if withNomadRow {
				addExecutorsRunningRow(t, repo, owner.ID, owner.TaskID, "nomad", `{}`)
			}
			member := &models.TaskSession{ID: "session-group-nomad-member", TaskID: "task-group-nomad-b"}
			err := repo.CreateTaskSessionWithSharedGroupWorkspaceBinding(ctx, member, &models.TaskEnvironment{
				TaskID: member.TaskID, ExecutorType: string(models.ExecutorTypeLocal),
			}, "group-nomad")
			if !withNomadRow {
				if err != nil {
					t.Fatalf("control shared bind without a Nomad row = %v, want re-election", err)
				}
				assertCreatingOwner(t, repo, owner.TaskEnvironmentID, member.ID)
				continue
			}
			if !errors.Is(err, models.ErrTaskEnvironmentResetRequired) {
				t.Fatalf("shared bind error = %v, want reset required", err)
			}
			requireRetained(t, repo, owner.TaskEnvironmentID)
		}
	})
	t.Run("ReelectFailedTaskEnvironmentMaterialization", func(t *testing.T) {
		repo, environmentID := seed(t, "task-failed-launch", true)
		reelected, err := repo.ReelectFailedTaskEnvironmentMaterialization(context.Background(), environmentID, "session-task-failed-launch-bound")
		if err != nil || reelected {
			t.Fatalf("reelect = (%v, %v), want (false, nil)", reelected, err)
		}
		requireRetained(t, repo, environmentID)

		repo, environmentID = seed(t, "task-failed-launch-control", false)
		reelected, err = repo.ReelectFailedTaskEnvironmentMaterialization(context.Background(), environmentID, "session-task-failed-launch-control-bound")
		if err != nil || !reelected {
			t.Fatalf("control reelect = (%v, %v), want (true, nil)", reelected, err)
		}
	})
}

func TestReelectNomad_OtherExecutorsUnchanged(t *testing.T) {
	t.Run("other runtimes do not block take-over", func(t *testing.T) {
		for _, runtime := range []string{"docker", "local", "kubernetes", ""} {
			repo := newRepoForEntityTests(t)
			seedMaterializationClaimTask(t, repo, "task-other-runtime")
			owner := seedCreatingOwner(t, repo, "task-other-runtime", "session-other-runtime-owner", models.TaskSessionStateCreated)
			ageTaskEnvironment(t, repo, owner.TaskEnvironmentID, 24*time.Hour)
			addExecutorsRunningRow(t, repo, owner.ID, owner.TaskID, runtime, `{}`)

			retry, err := bindRetry(repo, owner.TaskID, "session-other-runtime-retry")
			if err != nil {
				t.Fatalf("runtime %q: bind = %v, want take-over", runtime, err)
			}
			assertTakenOverMaterialization(t, repo, owner.TaskEnvironmentID, retry)
		}
	})
	t.Run("nomad row of another environment is not evidence", func(t *testing.T) {
		repo := newRepoForEntityTests(t)
		seedMaterializationClaimTask(t, repo, "task-nomad-elsewhere")
		owner := seedCreatingOwner(t, repo, "task-nomad-elsewhere", "session-nomad-elsewhere-owner", models.TaskSessionStateCreated)
		ageTaskEnvironment(t, repo, owner.TaskEnvironmentID, 24*time.Hour)
		seedMaterializationClaimTask(t, repo, "task-nomad-unrelated")
		unrelated := seedCreatingOwner(t, repo, "task-nomad-unrelated", "session-nomad-unrelated-owner", models.TaskSessionStateCreated)
		addExecutorsRunningRow(t, repo, unrelated.ID, unrelated.TaskID, "nomad", `{}`)

		retry, err := bindRetry(repo, owner.TaskID, "session-nomad-elsewhere-retry")
		if err != nil {
			t.Fatalf("bind = %v, want take-over", err)
		}
		assertTakenOverMaterialization(t, repo, owner.TaskEnvironmentID, retry)
	})
}
