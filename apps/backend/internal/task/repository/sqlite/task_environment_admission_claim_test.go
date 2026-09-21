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

type recoveryClaimSnapshot struct {
	Session     *models.TaskSession
	Environment *models.TaskEnvironment
	Claim       *models.TaskEnvironmentRecoveryClaim
}

func snapshotRecoveryClaim(t *testing.T, repo *Repository, sessionID, environmentID string) recoveryClaimSnapshot {
	t.Helper()
	session, err := repo.GetTaskSession(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession(%s): %v", sessionID, err)
	}
	environment, err := repo.GetTaskEnvironment(t.Context(), environmentID)
	if err != nil {
		t.Fatalf("GetTaskEnvironment(%s): %v", environmentID, err)
	}
	claim := &models.TaskEnvironmentRecoveryClaim{}
	if err := repo.db.QueryRowxContext(t.Context(), repo.db.Rebind(`
		SELECT task_environment_id, owner_task_id, ownership_generation, session_id,
			session_incarnation_id, operation_id, executor_type, created_at, updated_at
		FROM task_environment_recovery_claims WHERE task_environment_id = ?
	`), environmentID).Scan(
		&claim.TaskEnvironmentID, &claim.OwnerTaskID, &claim.OwnershipGeneration,
		&claim.SessionID, &claim.SessionIncarnationID, &claim.OperationID, &claim.ExecutorType,
		&claim.CreatedAt, &claim.UpdatedAt,
	); err != nil {
		t.Fatalf("read recovery claim: %v", err)
	}
	return recoveryClaimSnapshot{Session: session, Environment: environment, Claim: claim}
}

func assertRecoveryClaimSnapshot(t *testing.T, got, want recoveryClaimSnapshot) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("recovery claim state changed:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestCreateTurnWithStepStampRejectsForeignEnvironmentRecoveryClaim(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const (
		taskID        = "task-foreign-turn-claim"
		environmentID = "environment-foreign-turn-claim"
		requesterID   = "session-foreign-turn-requester"
		foreignID     = "session-foreign-turn-consumer"
	)
	seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
	for _, session := range []*models.TaskSession{
		{ID: requesterID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateCreated},
		{
			ID: foreignID, TaskID: taskID, TaskEnvironmentID: environmentID,
			QueueIncarnationID: "incarnation-foreign-turn-consumer",
			State:              models.TaskSessionStateWaitingForInput,
		},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: foreignID, SessionID: foreignID, TaskID: taskID,
		ExecutorID: "executor-foreign-turn", Status: models.ExecutorRunningStatusStopped,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(%s): %v", foreignID, err)
	}

	request := recoveryClaimRequest(t, repo, environmentID, taskID, requesterID, "operation-foreign-turn", 1)
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
	if err != nil {
		t.Fatalf("AcquireTaskEnvironmentRecoveryClaim: %v", err)
	}
	t.Cleanup(func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) })
	beforeSession, err := repo.GetTaskSession(ctx, foreignID)
	if err != nil {
		t.Fatalf("GetTaskSession before turn: %v", err)
	}

	turn := &models.Turn{ID: "turn-foreign-recovery-claim", TaskSessionID: foreignID, TaskID: taskID}
	stamped, err := repo.CreateTurnWithStepStamp(ctx, turn)
	if !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Errorf("CreateTurnWithStepStamp error = %v, want ErrBusy", err)
	}
	if stamped {
		t.Error("CreateTurnWithStepStamp stamped a foreign turn while the recovery claim was active")
	}
	if _, ok := turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart]; ok {
		t.Error("foreign turn retained a workflow-step stamp")
	}
	if _, getErr := repo.GetTurn(ctx, turn.ID); getErr == nil {
		t.Error("foreign turn was durably inserted")
	}

	afterSession, err := repo.GetTaskSession(ctx, foreignID)
	if err != nil {
		t.Fatalf("GetTaskSession after turn: %v", err)
	}
	if afterSession.ID != beforeSession.ID || afterSession.TaskID != beforeSession.TaskID ||
		afterSession.TaskEnvironmentID != beforeSession.TaskEnvironmentID ||
		afterSession.QueueIncarnationID != beforeSession.QueueIncarnationID ||
		afterSession.State != beforeSession.State || !afterSession.UpdatedAt.Equal(beforeSession.UpdatedAt) {
		t.Errorf("foreign session changed: before=%+v after=%+v", beforeSession, afterSession)
	}

	replayed, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
	if err != nil {
		t.Fatalf("replay recovery claim: %v", err)
	}
	if replayed.TaskEnvironmentID != claim.TaskEnvironmentID || replayed.OwnerTaskID != claim.OwnerTaskID ||
		replayed.OwnershipGeneration != claim.OwnershipGeneration || replayed.SessionID != claim.SessionID ||
		replayed.OperationID != claim.OperationID || replayed.ExecutorType != claim.ExecutorType ||
		!replayed.CreatedAt.Equal(claim.CreatedAt) || !replayed.UpdatedAt.Equal(claim.UpdatedAt) {
		t.Errorf("recovery claim changed: before=%+v after=%+v", claim, replayed)
	}
}

func TestTaskEnvironmentRecoveryClaimRejectsReplayedSessionIncarnation(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const (
		taskID        = "task-claim-incarnation"
		environmentID = "environment-claim-incarnation"
		sessionID     = "session-claim-incarnation"
	)
	seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	request := recoveryClaimRequest(t, repo, environmentID, taskID, sessionID, "operation-claim-incarnation", 1)
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
	if err != nil {
		t.Fatalf("AcquireTaskEnvironmentRecoveryClaim: %v", err)
	}
	t.Cleanup(func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) })

	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?
	`), "incarnation-claim-b", sessionID); err != nil {
		t.Fatalf("rotate session incarnation: %v", err)
	}
	before := snapshotRecoveryClaim(t, repo, sessionID, environmentID)

	replayed, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
	if !errors.Is(err, recoveryclaim.ErrClaimMismatch) {
		t.Errorf("replay error = %v, want ErrClaimMismatch", err)
	}
	if replayed != nil {
		t.Errorf("replayed claim = %+v, want nil", replayed)
	}

	turn := &models.Turn{ID: "turn-claim-incarnation", TaskSessionID: sessionID, TaskID: taskID}
	stamped, err := repo.CreateTurnWithStepStamp(recoveryclaim.WithClaim(ctx, claim), turn)
	if !errors.Is(err, recoveryclaim.ErrClaimMismatch) {
		t.Errorf("carried claim turn error = %v, want ErrClaimMismatch", err)
	}
	if stamped {
		t.Error("carried claim turn was stamped after requester incarnation changed")
	}
	if _, err := repo.GetTurn(ctx, turn.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetTurn after rejected carried claim = %v, want sql.ErrNoRows", err)
	}
	assertRecoveryClaimSnapshot(t, snapshotRecoveryClaim(t, repo, sessionID, environmentID), before)
}

func TestTaskEnvironmentRecoveryClaimLegacyIncarnationFailsClosed(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const (
		taskID        = "task-legacy-claim-incarnation"
		environmentID = "environment-legacy-claim-incarnation"
		sessionID     = "session-legacy-claim-incarnation"
	)
	seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `DROP TABLE task_environment_recovery_claims`); err != nil {
		t.Fatalf("drop final recovery claim table: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		CREATE TABLE task_environment_recovery_claims (
			task_environment_id TEXT PRIMARY KEY,
			owner_task_id TEXT NOT NULL,
			ownership_generation BIGINT NOT NULL,
			session_id TEXT NOT NULL,
			operation_id TEXT NOT NULL,
			executor_type TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (task_environment_id) REFERENCES task_environments(id) ON DELETE CASCADE
		)`); err != nil {
		t.Fatalf("create pre-incarnation recovery claim table: %v", err)
	}
	request := recoveryClaimRequest(t, repo, environmentID, taskID, sessionID, "operation-legacy-claim-incarnation", 1)
	now := time.Now().UTC()
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO task_environment_recovery_claims (
			task_environment_id, owner_task_id, ownership_generation, session_id,
			operation_id, executor_type, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), environmentID, taskID, 1, sessionID, request.OperationID, request.ExecutorType, now, now); err != nil {
		t.Fatalf("insert legacy claim: %v", err)
	}
	migrated, err := NewWithDB(repo.db, repo.ro, nil)
	if err != nil {
		t.Fatalf("migrate pre-incarnation recovery claim table: %v", err)
	}
	repo = migrated
	var legacyIncarnation string
	if err := repo.db.GetContext(ctx, &legacyIncarnation, repo.db.Rebind(`
		SELECT session_incarnation_id FROM task_environment_recovery_claims WHERE task_environment_id = ?
	`), environmentID); err != nil {
		t.Fatalf("read legacy claim incarnation: %v", err)
	}
	if legacyIncarnation != "" {
		t.Fatalf("legacy claim incarnation = %q, want empty", legacyIncarnation)
	}
	before := snapshotRecoveryClaim(t, repo, sessionID, environmentID)

	if replayed, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, request); !errors.Is(err, recoveryclaim.ErrBusy) || replayed != nil {
		t.Errorf("legacy replay = (%+v, %v), want (nil, ErrBusy)", replayed, err)
	}
	legacyClaim := &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID: environmentID, OwnerTaskID: taskID, OwnershipGeneration: 1,
		SessionID: sessionID, OperationID: request.OperationID, ExecutorType: request.ExecutorType,
		CreatedAt: now, UpdatedAt: now,
	}
	turn := &models.Turn{ID: "turn-legacy-claim-incarnation", TaskSessionID: sessionID, TaskID: taskID}
	if stamped, err := repo.CreateTurnWithStepStamp(recoveryclaim.WithClaim(ctx, legacyClaim), turn); !errors.Is(err, recoveryclaim.ErrClaimMismatch) || stamped {
		t.Errorf("legacy carried claim turn = (stamped=%t, %v), want (false, ErrClaimMismatch)", stamped, err)
	}
	if _, err := repo.GetTurn(ctx, turn.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetTurn after legacy rejection = %v, want sql.ErrNoRows", err)
	}
	assertRecoveryClaimSnapshot(t, snapshotRecoveryClaim(t, repo, sessionID, environmentID), before)
}

func TestTaskEnvironmentRecoveryClaimInactivePreserved(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const (
		taskID        = "task-admission-inactive"
		environmentID = "environment-admission-inactive"
		requesterID   = "session-admission-requester"
		preservedID   = "session-admission-preserved"
	)
	seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
	for _, session := range []*models.TaskSession{
		{ID: requesterID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateCreated},
		{ID: preservedID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateWaitingForInput},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: preservedID, SessionID: preservedID, TaskID: taskID,
		ExecutorID: "executor-admission", Status: models.ExecutorRunningStatusStopped,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(%s): %v", preservedID, err)
	}

	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx,
		recoveryClaimRequest(t, repo, environmentID, taskID, requesterID, "operation-admission-winner", 1))
	if err != nil {
		t.Fatalf("AcquireTaskEnvironmentRecoveryClaim: %v", err)
	}
	t.Cleanup(func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) })

	_, err = repo.AcquireTaskEnvironmentRecoveryClaim(ctx,
		recoveryClaimRequest(t, repo, environmentID, taskID, preservedID, "operation-admission-competitor", 1))
	if !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("competing claim error = %v, want ErrBusy", err)
	}
}

func TestTaskEnvironmentRecoveryClaimRejectsInvalidRequesterIdentity(t *testing.T) {
	tests := []struct {
		name             string
		missingRequester bool
		staleIncarnation bool
		seed             func(*testing.T, *Repository, string, string) string
	}{
		{name: "missing", missingRequester: true, seed: func(_ *testing.T, _ *Repository, _, _ string) string {
			return "session-requester-missing"
		}},
		{name: "wrong-task", seed: func(t *testing.T, repo *Repository, _, _ string) string {
			const (
				otherTaskID = "task-requester-other"
				otherEnvID  = "environment-requester-other"
				sessionID   = "session-requester-wrong-task"
			)
			seedRecoveryClaimEnvironment(t, repo, otherTaskID, otherEnvID)
			if err := repo.CreateTaskSession(t.Context(), &models.TaskSession{
				ID: sessionID, TaskID: otherTaskID, TaskEnvironmentID: otherEnvID,
				State: models.TaskSessionStateCreated,
			}); err != nil {
				t.Fatalf("CreateTaskSession: %v", err)
			}
			return sessionID
		}},
		{name: "wrong-environment", seed: func(t *testing.T, repo *Repository, taskID, _ string) string {
			const sessionID = "session-requester-wrong-environment"
			if err := repo.CreateTaskSession(t.Context(), &models.TaskSession{
				ID: sessionID, TaskID: taskID, State: models.TaskSessionStateCreated,
			}); err != nil {
				t.Fatalf("CreateTaskSession: %v", err)
			}
			return sessionID
		}},
		{name: "stale-incarnation", staleIncarnation: true, seed: func(t *testing.T, repo *Repository, taskID, environmentID string) string {
			const sessionID = "session-requester-stale-incarnation"
			if err := repo.CreateTaskSession(t.Context(), &models.TaskSession{
				ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
				State: models.TaskSessionStateCreated,
			}); err != nil {
				t.Fatalf("CreateTaskSession: %v", err)
			}
			return sessionID
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			const (
				taskID        = "task-requester-target"
				environmentID = "environment-requester-target"
			)
			seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
			sessionID := test.seed(t, repo, taskID, environmentID)
			request := models.TaskEnvironmentRecoveryClaimRequest{
				TaskEnvironmentID:    environmentID,
				OwnerTaskID:          taskID,
				OwnershipGeneration:  1,
				SessionID:            sessionID,
				SessionIncarnationID: "incarnation-requester-missing",
				OperationID:          "operation-requester-" + test.name,
				ExecutorType:         string(models.ExecutorTypeWorktree),
			}
			if !test.missingRequester {
				request = recoveryClaimRequest(t, repo, environmentID, taskID, sessionID, "operation-requester-"+test.name, 1)
			}
			var beforeSession *models.TaskSession
			var err error
			if test.staleIncarnation {
				if _, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`
					UPDATE task_sessions SET queue_incarnation_id = '' WHERE id = ?
				`), sessionID); err != nil {
					t.Fatalf("clear requester incarnation: %v", err)
				}
				beforeSession, err = repo.GetTaskSession(t.Context(), sessionID)
				if err != nil {
					t.Fatalf("GetTaskSession before stale replay: %v", err)
				}
			}

			claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(t.Context(), request)
			if !errors.Is(err, recoveryclaim.ErrClaimMismatch) {
				t.Fatalf("AcquireTaskEnvironmentRecoveryClaim error = %v, want ErrClaimMismatch", err)
			}
			if claim != nil {
				t.Fatalf("claim = %+v, want nil", claim)
			}
			var count int
			if err := repo.db.GetContext(t.Context(), &count, repo.db.Rebind(`
				SELECT count(*) FROM task_environment_recovery_claims WHERE task_environment_id = ?
			`), environmentID); err != nil {
				t.Fatalf("count recovery claims: %v", err)
			}
			if count != 0 {
				t.Fatalf("recovery claim count = %d, want 0", count)
			}
			if beforeSession != nil {
				afterSession, err := repo.GetTaskSession(t.Context(), sessionID)
				if err != nil {
					t.Fatalf("GetTaskSession after stale replay: %v", err)
				}
				if !reflect.DeepEqual(afterSession, beforeSession) {
					t.Errorf("requester session changed after stale replay:\n got: %#v\nwant: %#v", afterSession, beforeSession)
				}
			}
		})
	}
}

type sessionAdmissionSurface struct {
	name   string
	mutate func(context.Context, *Repository, string) error
}

func sessionAdmissionSurfaces() []sessionAdmissionSurface {
	return []sessionAdmissionSurface{
		{name: "state", mutate: func(ctx context.Context, repo *Repository, sessionID string) error {
			return repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateRunning, "")
		}},
		{name: "state-cas", mutate: func(ctx context.Context, repo *Repository, sessionID string) error {
			changed, _, err := repo.UpdateTaskSessionStateIfCurrent(
				ctx, sessionID, models.TaskSessionStateWaitingForInput, models.TaskSessionStateRunning, "",
			)
			if err == nil && !changed {
				return errors.New("state CAS did not update")
			}
			return err
		}},
		{name: "state-identity-cas", mutate: func(ctx context.Context, repo *Repository, sessionID string) error {
			session, err := repo.GetTaskSession(ctx, sessionID)
			if err != nil {
				return err
			}
			changed, _, err := repo.UpdateTaskSessionStateIfCurrentIdentity(
				ctx, session.TaskID, session.ID, session.QueueIncarnationID,
				models.TaskSessionStateWaitingForInput, models.TaskSessionStateRunning, "",
			)
			if err == nil && !changed {
				return errors.New("identity state CAS did not update")
			}
			return err
		}},
		{name: "promptable", mutate: func(ctx context.Context, repo *Repository, sessionID string) error {
			claim, err := repo.ClaimPromptableTaskSessionIfActive(ctx, sessionID)
			if err == nil && claim.Status != models.PromptableTaskSessionClaimed {
				return errors.New("promptable session was not claimed")
			}
			return err
		}},
		{name: "promptable-identity", mutate: func(ctx context.Context, repo *Repository, sessionID string) error {
			session, err := repo.GetTaskSession(ctx, sessionID)
			if err != nil {
				return err
			}
			claim, err := repo.ClaimPromptableTaskSessionIfActiveForIdentity(
				ctx, session.TaskID, session.ID, session.QueueIncarnationID,
			)
			if err == nil && claim.Status != models.PromptableTaskSessionClaimed {
				return errors.New("identity promptable session was not claimed")
			}
			return err
		}},
		{name: "full-row", mutate: func(ctx context.Context, repo *Repository, sessionID string) error {
			session, err := repo.GetTaskSession(ctx, sessionID)
			if err != nil {
				return err
			}
			session.State = models.TaskSessionStateRunning
			session.CompletedAt = nil
			return repo.UpdateTaskSession(ctx, session)
		}},
	}
}

func TestSessionAdmissionSurfacesRespectEnvironmentRecoveryClaim(t *testing.T) {
	for _, surface := range sessionAdmissionSurfaces() {
		for _, mode := range []struct {
			name         string
			targetHolder bool
			carryClaim   bool
			wantBusy     bool
		}{
			{name: "claim-holder", targetHolder: true, carryClaim: true},
			{name: "foreign", wantBusy: true},
			{name: "carried-claim-foreign-target", carryClaim: true, wantBusy: true},
		} {
			t.Run(surface.name+"/"+mode.name, func(t *testing.T) {
				repo, ctx, claim, holderID, foreignID := seedSessionAdmissionClaim(t, surface.name+"-"+mode.name)
				targetID := foreignID
				if mode.targetHolder {
					targetID = holderID
				}
				if mode.carryClaim {
					ctx = recoveryclaim.WithClaim(ctx, claim)
				}
				before, err := repo.GetTaskSession(ctx, targetID)
				if err != nil {
					t.Fatalf("GetTaskSession before mutation: %v", err)
				}

				err = surface.mutate(ctx, repo, targetID)
				if mode.wantBusy {
					if !errors.Is(err, recoveryclaim.ErrBusy) {
						t.Fatalf("mutation error = %v, want ErrBusy", err)
					}
					after, getErr := repo.GetTaskSession(t.Context(), targetID)
					if getErr != nil {
						t.Fatalf("GetTaskSession after rejected mutation: %v", getErr)
					}
					if after.State != before.State || after.TaskID != before.TaskID ||
						after.TaskEnvironmentID != before.TaskEnvironmentID ||
						after.QueueIncarnationID != before.QueueIncarnationID ||
						!after.UpdatedAt.Equal(before.UpdatedAt) {
						t.Fatalf("rejected session changed: before=%+v after=%+v", before, after)
					}
					return
				}
				if err != nil {
					t.Fatalf("claim-holder mutation: %v", err)
				}
				after, getErr := repo.GetTaskSession(t.Context(), targetID)
				if getErr != nil {
					t.Fatalf("GetTaskSession after mutation: %v", getErr)
				}
				if after.State != models.TaskSessionStateRunning {
					t.Fatalf("session state = %s, want RUNNING", after.State)
				}
			})
		}
	}
}

func seedSessionAdmissionClaim(
	t *testing.T,
	suffix string,
) (*Repository, context.Context, *models.TaskEnvironmentRecoveryClaim, string, string) {
	t.Helper()
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	taskID := "task-state-admission-" + suffix
	environmentID := "environment-state-admission-" + suffix
	holderID := "session-state-holder-" + suffix
	foreignID := "session-state-foreign-" + suffix
	seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
	for _, session := range []*models.TaskSession{
		{ID: holderID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateWaitingForInput},
		{ID: foreignID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateWaitingForInput},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: foreignID, SessionID: foreignID, TaskID: taskID,
		ExecutorID: "executor-state-foreign-" + suffix, Status: models.ExecutorRunningStatusStopped,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(%s): %v", foreignID, err)
	}
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx,
		recoveryClaimRequest(t, repo, environmentID, taskID, holderID, "operation-state-admission-"+suffix, 1))
	if err != nil {
		t.Fatalf("AcquireTaskEnvironmentRecoveryClaim: %v", err)
	}
	t.Cleanup(func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) })
	return repo, ctx, claim, holderID, foreignID
}

func TestTaskEnvironmentRecoveryClaimExecutorRunningBlocks(t *testing.T) {
	repo, ctx, request := seedAdmissionClaimBlocker(t, "executor-running", models.TaskSessionStateWaitingForInput)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-executor-running", SessionID: "session-executor-running", TaskID: "task-executor-running",
		ExecutorID: "executor-admission", Status: models.ExecutorRunningStatusRunning,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	assertAdmissionClaimBusy(t, repo, ctx, request)
}

func TestTaskEnvironmentRecoveryClaimOpenTurnBlocks(t *testing.T) {
	repo, ctx, request := seedAdmissionClaimBlocker(t, "open-turn", models.TaskSessionStateWaitingForInput)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-open", TaskSessionID: "session-open-turn", TaskID: "task-open-turn",
	}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	assertAdmissionClaimBusy(t, repo, ctx, request)
}

func TestTaskEnvironmentRecoveryClaimSessionStartingRunningBlocks(t *testing.T) {
	for _, state := range []models.TaskSessionState{models.TaskSessionStateStarting, models.TaskSessionStateRunning} {
		t.Run(string(state), func(t *testing.T) {
			suffix := "session-" + string(state)
			repo, ctx, request := seedAdmissionClaimBlocker(t, suffix, state)
			assertAdmissionClaimBusy(t, repo, ctx, request)
		})
	}
}

func TestTaskEnvironmentRecoveryClaimMaterializationBlocks(t *testing.T) {
	repo, ctx, request := seedAdmissionClaimBlocker(t, "materialization", models.TaskSessionStateWaitingForInput)
	if err := repo.UpdateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "environment-materialization", TaskID: "task-materialization",
		ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: "session-materialization",
	}); err != nil {
		t.Fatalf("UpdateTaskEnvironment: %v", err)
	}
	assertAdmissionClaimBusy(t, repo, ctx, request)
}

func seedAdmissionClaimBlocker(
	t *testing.T,
	suffix string,
	state models.TaskSessionState,
) (*Repository, context.Context, models.TaskEnvironmentRecoveryClaimRequest) {
	t.Helper()
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	taskID := "task-" + suffix
	environmentID := "environment-" + suffix
	requesterID := "requester-" + suffix
	seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
	for _, session := range []*models.TaskSession{
		{ID: requesterID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateCreated},
		{ID: "session-" + suffix, TaskID: taskID, TaskEnvironmentID: environmentID, State: state},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
	}
	return repo, ctx, recoveryClaimRequest(t, repo, environmentID, taskID, requesterID, "operation-"+suffix, 1)
}

func assertAdmissionClaimBusy(
	t *testing.T,
	repo *Repository,
	ctx context.Context,
	request models.TaskEnvironmentRecoveryClaimRequest,
) {
	t.Helper()
	_, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
	if !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("AcquireTaskEnvironmentRecoveryClaim error = %v, want ErrBusy", err)
	}
}
