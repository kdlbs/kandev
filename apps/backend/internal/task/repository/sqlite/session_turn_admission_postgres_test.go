package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

func TestPostgresRecoveryClaimTaskLockOrdersBeforeTurnWriter(t *testing.T) {
	repoA, repoB, observer := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	const (
		taskID        = "task-claim-before-turn"
		environmentID = "environment-claim-before-turn"
		requesterID   = "session-claim-before-turn"
		foreignID     = "session-turn-after-claim"
	)
	seedTurnAdmissionRace(t, repoA, taskID, environmentID, requesterID, foreignID)

	claim := recoveryClaimRequest(t, repoA, environmentID, taskID, requesterID, "operation-claim-before-turn", 1)
	holder, err := repoA.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin claim transaction: %v", err)
	}
	defer func() { _ = holder.Rollback() }()
	if err := kandevdb.LockTaskRowInTx(ctx, holder, repoA.db.DriverName(), taskID); err != nil {
		t.Fatalf("lock task for claim: %v", err)
	}
	now := time.Now().UTC()
	if _, err := holder.ExecContext(ctx, repoA.db.Rebind(`
		INSERT INTO task_environment_recovery_claims (
			task_environment_id, owner_task_id, ownership_generation, session_id,
			operation_id, executor_type, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), claim.TaskEnvironmentID, claim.OwnerTaskID, claim.OwnershipGeneration,
		claim.SessionID, claim.OperationID, claim.ExecutorType, now, now); err != nil {
		t.Fatalf("insert recovery claim: %v", err)
	}

	writerPID := pgBackendPID(t, repoB.db)
	turn := &models.Turn{ID: "turn-after-claim", TaskID: taskID, TaskSessionID: foreignID}
	writerDone := make(chan struct{})
	var stamped bool
	var writerErr error
	go func() {
		stamped, writerErr = repoB.CreateTurnWithStepStamp(ctx, turn)
		close(writerDone)
	}()
	t.Cleanup(func() {
		_ = holder.Rollback()
		select {
		case <-writerDone:
		case <-time.After(5 * time.Second):
			t.Errorf("timed out waiting for turn writer during cleanup")
		}
	})

	if err := waitForPostgresLock(ctx, observer, writerPID, writerDone); err != nil {
		t.Fatal(err)
	}
	select {
	case <-writerDone:
		t.Fatal("turn writer returned while claim transaction held the task lock")
	default:
	}
	if err := holder.Commit(); err != nil {
		t.Fatalf("commit claim transaction: %v", err)
	}
	select {
	case <-writerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for turn writer after claim commit")
	}
	if !errors.Is(writerErr, recoveryclaim.ErrBusy) {
		t.Fatalf("turn writer error = %v, want ErrBusy", writerErr)
	}
	if stamped {
		t.Error("turn writer stamped a rejected turn")
	}
	if _, err := repoB.GetTurn(ctx, turn.ID); err == nil {
		t.Error("turn writer inserted a turn after the recovery claim committed")
	}
}

func TestPostgresTurnWriterTaskLockOrdersBeforeRecoveryClaim(t *testing.T) {
	repoA, repoB, observer := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	const (
		taskID        = "task-turn-before-claim"
		environmentID = "environment-turn-before-claim"
		requesterID   = "session-claim-after-turn"
		foreignID     = "session-turn-before-claim"
	)
	seedTurnAdmissionRace(t, repoA, taskID, environmentID, requesterID, foreignID)

	writer, err := repoA.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin turn transaction: %v", err)
	}
	defer func() { _ = writer.Rollback() }()
	if _, err := repoA.admitSessionWriteTx(ctx, writer, foreignID); err != nil {
		t.Fatalf("admit turn writer: %v", err)
	}
	turn := &models.Turn{ID: "turn-before-claim", TaskID: taskID, TaskSessionID: foreignID}
	stampTurnDefaults(turn)
	if err := repoA.insertTurnRow(ctx, writer, turn); err != nil {
		t.Fatalf("insert turn in held writer transaction: %v", err)
	}

	claimPID := pgBackendPID(t, repoB.db)
	claimDone := make(chan struct{})
	var claimErr error
	go func() {
		_, claimErr = repoB.AcquireTaskEnvironmentRecoveryClaim(ctx,
			recoveryClaimRequest(t, repoB, environmentID, taskID, requesterID, "operation-after-turn", 1))
		close(claimDone)
	}()
	t.Cleanup(func() {
		_ = writer.Rollback()
		select {
		case <-claimDone:
		case <-time.After(5 * time.Second):
			t.Errorf("timed out waiting for recovery claimant during cleanup")
		}
	})

	if err := waitForPostgresLock(ctx, observer, claimPID, claimDone); err != nil {
		t.Fatal(err)
	}
	select {
	case <-claimDone:
		t.Fatal("recovery claimant returned while turn transaction held the task lock")
	default:
	}
	if err := writer.Commit(); err != nil {
		t.Fatalf("commit turn transaction: %v", err)
	}
	select {
	case <-claimDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for recovery claimant after turn commit")
	}
	if !errors.Is(claimErr, recoveryclaim.ErrBusy) {
		t.Fatalf("recovery claim error = %v, want ErrBusy", claimErr)
	}
	if _, err := repoA.GetTurn(ctx, turn.ID); err != nil {
		t.Fatalf("committed turn missing: %v", err)
	}
	var claims int
	if err := repoA.db.GetContext(ctx, &claims,
		repoA.db.Rebind(`SELECT count(*) FROM task_environment_recovery_claims WHERE task_environment_id = ?`),
		environmentID); err != nil {
		t.Fatalf("count recovery claims: %v", err)
	}
	if claims != 0 {
		t.Fatalf("recovery claim count = %d, want 0", claims)
	}
}

func TestPostgresTurnWriterRejectsIncarnationChangeAfterSessionLock(t *testing.T) {
	repoA, repoB, observer := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	const (
		taskID        = "task-incarnation-after-session-lock"
		environmentID = "environment-incarnation-after-session-lock"
		requesterID   = "session-incarnation-requester"
		targetID      = "session-incarnation-target"
		turnID        = "turn-incarnation-after-session-lock"
	)
	seedTurnAdmissionRace(t, repoA, taskID, environmentID, requesterID, targetID)

	sessionHolder, err := repoA.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin session-lock holder: %v", err)
	}
	defer func() { _ = sessionHolder.Rollback() }()
	if err := lockSessionTurnWrites(ctx, sessionHolder, repoA.db.DriverName(), targetID); err != nil {
		t.Fatalf("lock target session: %v", err)
	}

	writerPID := pgBackendPID(t, repoB.db)
	turn := &models.Turn{ID: turnID, TaskID: taskID, TaskSessionID: targetID}
	writerDone := make(chan struct{})
	var writerErr error
	go func() {
		writerErr = repoB.CreateTurn(ctx, turn)
		close(writerDone)
	}()
	t.Cleanup(func() {
		_ = sessionHolder.Rollback()
		select {
		case <-writerDone:
		case <-time.After(5 * time.Second):
			t.Errorf("timed out waiting for turn writer during cleanup")
		}
	})

	if err := waitForPostgresLock(ctx, observer, writerPID, writerDone); err != nil {
		t.Fatal(err)
	}
	if _, err := observer.ExecContext(ctx, observer.Rebind(`
		UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?
	`), "incarnation-replaced-after-session-lock", targetID); err != nil {
		t.Fatalf("replace session incarnation: %v", err)
	}
	if err := sessionHolder.Commit(); err != nil {
		t.Fatalf("commit session-lock holder: %v", err)
	}
	select {
	case <-writerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for turn writer after session lock release")
	}
	if writerErr == nil {
		t.Fatal("turn writer succeeded after the target session incarnation changed")
	}
	if _, err := repoB.GetTurn(ctx, turnID); err == nil {
		t.Error("turn writer inserted a turn for the replaced session incarnation")
	}
}

func seedTurnAdmissionRace(
	t *testing.T,
	repo *Repository,
	taskID, environmentID, requesterID, foreignID string,
) {
	t.Helper()
	ctx := context.Background()
	seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
	for _, session := range []*models.TaskSession{
		{ID: requesterID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateCreated},
		{
			ID: foreignID, TaskID: taskID, TaskEnvironmentID: environmentID,
			QueueIncarnationID: "incarnation-" + foreignID,
			State:              models.TaskSessionStateWaitingForInput,
		},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: foreignID, SessionID: foreignID, TaskID: taskID,
		ExecutorID: "executor-" + foreignID, Status: models.ExecutorRunningStatusStopped,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(%s): %v", foreignID, err)
	}
}
