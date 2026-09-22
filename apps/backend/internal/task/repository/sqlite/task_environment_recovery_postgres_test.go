package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

func TestPostgresTaskEnvironmentRecoveryClaimReplayBlocksLiveConsumers(t *testing.T) {
	for _, test := range []struct {
		name string
		seed func(t *testing.T, repo *Repository, taskID, sessionID, environmentID string)
	}{
		{name: "prelaunch"},
		{name: "requester turn", seed: func(t *testing.T, repo *Repository, taskID, sessionID, _ string) {
			now := time.Now().UTC()
			if _, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`), "turn-live-requester-pg", sessionID, taskID, now, now, now); err != nil {
				t.Fatalf("seed requester turn: %v", err)
			}
		}},
		{name: "attached consumer turn", seed: func(t *testing.T, repo *Repository, taskID, _ string, environmentID string) {
			now := time.Now().UTC()
			if err := repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "session-live-consumer-pg", TaskID: taskID, TaskEnvironmentID: environmentID, QueueIncarnationID: "incarnation-live-consumer-pg", State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now}); err != nil {
				t.Fatalf("seed consumer: %v", err)
			}
			if _, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`), "turn-live-consumer-pg", "session-live-consumer-pg", taskID, now, now, now); err != nil {
				t.Fatalf("seed consumer turn: %v", err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repoA, repoB, _ := newTaskPostgresRepoPair(t)
			const taskID, environmentID, sessionID = "task-live-replay-pg", "environment-live-replay-pg", "session-live-replay-pg"
			seedRecoveryClaimEnvironment(t, repoA, taskID, environmentID)
			if err := repoA.CreateTaskSession(t.Context(), &models.TaskSession{ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateCreated}); err != nil {
				t.Fatalf("create requester: %v", err)
			}
			request := recoveryClaimRequest(t, repoA, environmentID, taskID, sessionID, "operation-live-replay-pg", 1)
			claim, err := repoA.AcquireTaskEnvironmentRecoveryClaim(t.Context(), request)
			if err != nil {
				t.Fatalf("acquire: %v", err)
			}
			t.Cleanup(func() { _ = repoA.ReleaseTaskEnvironmentRecoveryClaim(t.Context(), claim) })
			if test.seed != nil {
				test.seed(t, repoB, taskID, sessionID, environmentID)
			}
			before := snapshotPostgresRecoveryClaim(t, repoA, sessionID, environmentID)
			replayed, err := repoA.AcquireTaskEnvironmentRecoveryClaim(t.Context(), request)
			if test.seed == nil {
				if err != nil || !reflect.DeepEqual(replayed, claim) {
					t.Fatalf("prelaunch replay = (%+v, %v), want unchanged claim", replayed, err)
				}
			} else if !errors.Is(err, recoveryclaim.ErrBusy) || replayed != nil {
				t.Fatalf("live replay = (%+v, %v), want (nil, ErrBusy)", replayed, err)
			}
			if after := snapshotPostgresRecoveryClaim(t, repoA, sessionID, environmentID); !reflect.DeepEqual(after, before) {
				t.Fatalf("claim changed: got %#v want %#v", after, before)
			}
		})
	}
}

func TestPostgresTaskEnvironmentRecoveryClaimLegacyIncarnationMigrationFailsClosed(t *testing.T) {
	repo, _, _ := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	const taskID, environmentID, sessionID = "task-legacy-claim-pg", "environment-legacy-claim-pg", "session-legacy-claim-pg"
	seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateCreated}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `DROP TABLE task_environment_recovery_claims`); err != nil {
		t.Fatalf("drop claim table: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `CREATE TABLE task_environment_recovery_claims (task_environment_id TEXT PRIMARY KEY, owner_task_id TEXT NOT NULL, ownership_generation BIGINT NOT NULL, session_id TEXT NOT NULL, operation_id TEXT NOT NULL, executor_type TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	request := recoveryClaimRequest(t, repo, environmentID, taskID, sessionID, "operation-legacy-claim-pg", 1)
	now := time.Now().UTC()
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`INSERT INTO task_environment_recovery_claims (task_environment_id, owner_task_id, ownership_generation, session_id, operation_id, executor_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`), environmentID, taskID, 1, sessionID, request.OperationID, request.ExecutorType, now, now); err != nil {
		t.Fatalf("insert legacy claim: %v", err)
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("migrate legacy table: %v", err)
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("replay migration: %v", err)
	}
	var incarnation string
	if err := repo.db.GetContext(ctx, &incarnation, repo.db.Rebind(`SELECT session_incarnation_id FROM task_environment_recovery_claims WHERE task_environment_id = ?`), environmentID); err != nil {
		t.Fatalf("read legacy incarnation: %v", err)
	}
	if incarnation != "" {
		t.Fatalf("legacy incarnation = %q, want empty", incarnation)
	}
	if replayed, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, request); !errors.Is(err, recoveryclaim.ErrBusy) || replayed != nil {
		t.Fatalf("legacy replay = (%+v, %v), want (nil, ErrBusy)", replayed, err)
	}
}

func TestPostgresTaskEnvironmentRecoveryClaimInheritedAuthorization(t *testing.T) {
	for _, test := range []struct {
		name, requesterWorkspace, parentID string
		wantErr                            error
	}{
		{name: "authorized child", requesterWorkspace: "workspace-authorized-pg", parentID: "task-owner-pg"},
		{name: "unrelated task", requesterWorkspace: "workspace-authorized-pg", wantErr: recoveryclaim.ErrClaimMismatch},
		{name: "cross workspace child", requesterWorkspace: "workspace-other-pg", parentID: "task-owner-pg", wantErr: recoveryclaim.ErrClaimMismatch},
	} {
		t.Run(test.name, func(t *testing.T) {
			repoA, repoB, _ := newTaskPostgresRepoPair(t)
			ctx := t.Context()
			if err := repoA.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-authorized-pg", Name: "Authorized"}); err != nil {
				t.Fatal(err)
			}
			if test.requesterWorkspace != "workspace-authorized-pg" {
				if err := repoA.CreateWorkspace(ctx, &models.Workspace{ID: test.requesterWorkspace, Name: "Other"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := repoA.CreateTask(ctx, &models.Task{ID: "task-owner-pg", WorkspaceID: "workspace-authorized-pg", Title: "Owner"}); err != nil {
				t.Fatal(err)
			}
			if err := repoA.CreateTask(ctx, &models.Task{ID: "task-requester-pg", WorkspaceID: test.requesterWorkspace, ParentID: test.parentID, Title: "Requester", Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}}}); err != nil {
				t.Fatal(err)
			}
			if err := repoA.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "environment-authorized-pg", TaskID: "task-owner-pg", ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusCreating, OwnershipGeneration: 1}); err != nil {
				t.Fatal(err)
			}
			if err := repoA.CreateTaskSession(ctx, &models.TaskSession{ID: "session-authorized-pg", TaskID: "task-requester-pg", TaskEnvironmentID: "environment-authorized-pg", QueueIncarnationID: "incarnation-authorized-pg", State: models.TaskSessionStateCreated}); err != nil {
				t.Fatal(err)
			}
			request := recoveryClaimRequest(t, repoA, "environment-authorized-pg", "task-owner-pg", "session-authorized-pg", "operation-"+test.name, 1)
			claim, err := repoB.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) || claim != nil {
					t.Fatalf("acquire = (%+v, %v), want (nil, %v)", claim, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("authorized acquire: %v", err)
			}
			t.Cleanup(func() { _ = repoA.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) })
			if replayed, err := repoA.AcquireTaskEnvironmentRecoveryClaim(ctx, request); err != nil || !reflect.DeepEqual(replayed, claim) {
				t.Fatalf("authorized replay = (%+v, %v), want unchanged", replayed, err)
			}
		})
	}
}

func TestPostgresTaskEnvironmentRecoveryClaimSharedGroupAuthorization(t *testing.T) {
	repoA, repoB, _ := newTaskPostgresRepoPair(t)
	ctx := t.Context()
	for _, statement := range []string{
		`CREATE TABLE task_workspace_groups (id TEXT PRIMARY KEY, owner_task_id TEXT NOT NULL, materialized_environment_id TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE task_workspace_group_members (workspace_group_id TEXT NOT NULL, task_id TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'member', released_at TIMESTAMP, created_at TIMESTAMP NOT NULL, PRIMARY KEY (workspace_group_id, task_id))`,
	} {
		if _, err := repoA.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("create workspace group table: %v", err)
		}
	}
	if err := repoA.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-group-pg", Name: "Group"}); err != nil {
		t.Fatal(err)
	}
	for _, task := range []*models.Task{{ID: "task-group-owner-pg", WorkspaceID: "workspace-group-pg", Title: "Owner"}, {ID: "task-group-member-pg", WorkspaceID: "workspace-group-pg", Title: "Member", Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "shared_group"}}}} {
		if err := repoA.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	if err := repoA.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "environment-group-pg", TaskID: "task-group-owner-pg", ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusCreating, OwnershipGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := repoA.db.ExecContext(ctx, repoA.db.Rebind(`INSERT INTO task_workspace_groups (id, owner_task_id, materialized_environment_id) VALUES (?, ?, ?)`), "group-pg", "task-group-owner-pg", "environment-group-pg"); err != nil {
		t.Fatal(err)
	}
	if _, err := repoA.db.ExecContext(ctx, repoA.db.Rebind(`INSERT INTO task_workspace_group_members (workspace_group_id, task_id, created_at) VALUES (?, ?, ?)`), "group-pg", "task-group-member-pg", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := repoA.CreateTaskSession(ctx, &models.TaskSession{ID: "session-group-pg", TaskID: "task-group-member-pg", TaskEnvironmentID: "environment-group-pg", QueueIncarnationID: "incarnation-group-pg", State: models.TaskSessionStateCreated}); err != nil {
		t.Fatal(err)
	}
	request := recoveryClaimRequest(t, repoA, "environment-group-pg", "task-group-owner-pg", "session-group-pg", "operation-group-pg", 1)
	claim, err := repoB.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
	if err != nil {
		t.Fatalf("shared group acquire: %v", err)
	}
	t.Cleanup(func() { _ = repoA.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) })
	if replayed, err := repoA.AcquireTaskEnvironmentRecoveryClaim(ctx, request); err != nil || !reflect.DeepEqual(replayed, claim) {
		t.Fatalf("shared group replay = (%+v, %v), want unchanged", replayed, err)
	}
}

func TestPostgresTaskEnvironmentRecoveryClaimRejectsReplayedSessionIncarnation(t *testing.T) {
	repoA, repoB, _ := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	const (
		taskID        = "task-postgres-claim-incarnation"
		environmentID = "environment-postgres-claim-incarnation"
		sessionID     = "session-postgres-claim-incarnation"
	)
	seedRecoveryClaimEnvironment(t, repoA, taskID, environmentID)
	if err := repoA.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	request := recoveryClaimRequest(t, repoA, environmentID, taskID, sessionID, "operation-postgres-claim-incarnation", 1)
	claim, err := repoA.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
	if err != nil {
		t.Fatalf("AcquireTaskEnvironmentRecoveryClaim: %v", err)
	}
	t.Cleanup(func() { _ = repoA.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) })
	if _, err := repoB.db.ExecContext(ctx, repoB.db.Rebind(`
		UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?
	`), "incarnation-postgres-b", sessionID); err != nil {
		t.Fatalf("rotate session incarnation: %v", err)
	}
	before := snapshotPostgresRecoveryClaim(t, repoA, sessionID, environmentID)

	if replayed, err := repoA.AcquireTaskEnvironmentRecoveryClaim(ctx, request); !errors.Is(err, recoveryclaim.ErrClaimMismatch) || replayed != nil {
		t.Errorf("replay = (%+v, %v), want (nil, ErrClaimMismatch)", replayed, err)
	}
	turn := &models.Turn{ID: "turn-postgres-claim-incarnation", TaskSessionID: sessionID, TaskID: taskID}
	if stamped, err := repoB.CreateTurnWithStepStamp(recoveryclaim.WithClaim(ctx, claim), turn); !errors.Is(err, recoveryclaim.ErrClaimMismatch) || stamped {
		t.Errorf("carried turn = (stamped=%t, %v), want (false, ErrClaimMismatch)", stamped, err)
	}
	if _, err := repoB.GetTurn(ctx, turn.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetTurn after rejected carried claim = %v, want sql.ErrNoRows", err)
	}
	if after := snapshotPostgresRecoveryClaim(t, repoA, sessionID, environmentID); !reflect.DeepEqual(after, before) {
		t.Errorf("Postgres recovery claim state changed:\n got: %#v\nwant: %#v", after, before)
	}
}

func snapshotPostgresRecoveryClaim(t *testing.T, repo *Repository, sessionID, environmentID string) recoveryClaimSnapshot {
	t.Helper()
	return snapshotRecoveryClaim(t, repo, sessionID, environmentID)
}

func TestPostgresTaskEnvironmentRecoveryClaimSerializesIndependentRepositories(t *testing.T) {
	repoA, repoB, _ := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	const (
		taskID        = "task-postgres-recovery-claim"
		environmentID = "environment-postgres-recovery-claim"
	)
	seedRecoveryClaimEnvironment(t, repoA, taskID, environmentID)
	for _, sessionID := range []string{"session-postgres-recovery-a", "session-postgres-recovery-b"} {
		if err := repoA.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
			State: models.TaskSessionStateWaitingForInput,
		}); err != nil {
			t.Fatalf("create recovery session %s: %v", sessionID, err)
		}
		if err := repoA.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: sessionID, SessionID: sessionID, TaskID: taskID,
			ExecutorID: "executor-" + sessionID, Status: models.ExecutorRunningStatusStopped,
		}); err != nil {
			t.Fatalf("create stopped executor for %s: %v", sessionID, err)
		}
	}

	requestA := recoveryClaimRequest(t, repoA, environmentID, taskID, "session-postgres-recovery-a", "operation-postgres-recovery-a", 1)
	claimA, err := repoA.AcquireTaskEnvironmentRecoveryClaim(ctx, requestA)
	if err != nil {
		t.Fatalf("acquire recovery claim from repository A: %v", err)
	}

	requestB := recoveryClaimRequest(t, repoB, environmentID, taskID, "session-postgres-recovery-b", "operation-postgres-recovery-b", 1)
	if _, err := repoB.AcquireTaskEnvironmentRecoveryClaim(ctx, requestB); !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("competing claim from repository B error = %v, want ErrBusy", err)
	}

	if err := repoB.ReleaseTaskEnvironmentRecoveryClaim(ctx, claimA); err != nil {
		t.Fatalf("release recovery claim from repository B: %v", err)
	}
	claimB, err := repoB.AcquireTaskEnvironmentRecoveryClaim(ctx, requestB)
	if err != nil {
		t.Fatalf("acquire recovery claim after release: %v", err)
	}
	if err := repoA.ReleaseTaskEnvironmentRecoveryClaim(ctx, claimB); err != nil {
		t.Fatalf("release replacement recovery claim from repository A: %v", err)
	}
}
