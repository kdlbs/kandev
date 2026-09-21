package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

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
