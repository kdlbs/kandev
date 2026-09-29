package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresSessionAdmissionWritersWaitForClaimAndReread(t *testing.T) {
	for _, surface := range sessionAdmissionSurfaces() {
		t.Run(surface.name, func(t *testing.T) {
			repoA, repoB, observer := newTaskPostgresRepoPair(t)
			ctx := context.Background()
			taskID := "task-state-claim-before-" + surface.name
			environmentID := "environment-state-claim-before-" + surface.name
			holderID := "session-state-holder-" + surface.name
			targetID := "session-state-target-" + surface.name
			seedTurnAdmissionRace(t, repoA, taskID, environmentID, holderID, targetID)

			claim := recoveryClaimRequest(t, repoA, environmentID, taskID, holderID, "operation-state-claim-before-"+surface.name, 1)
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
			writerDone := make(chan struct{})
			var writerErr error
			go func() {
				writerErr = surface.mutate(ctx, repoB, targetID)
				close(writerDone)
			}()
			t.Cleanup(func() {
				_ = holder.Rollback()
				select {
				case <-writerDone:
				case <-time.After(5 * time.Second):
					t.Errorf("timed out waiting for %s writer during cleanup", surface.name)
				}
			})

			if err := waitForPostgresLock(ctx, observer, writerPID, writerDone); err != nil {
				t.Fatal(err)
			}
			select {
			case <-writerDone:
				t.Fatalf("%s writer returned while claim transaction held the task lock", surface.name)
			default:
			}
			if err := holder.Commit(); err != nil {
				t.Fatalf("commit claim transaction: %v", err)
			}
			select {
			case <-writerDone:
			case <-time.After(5 * time.Second):
				t.Fatalf("timed out waiting for %s writer after claim commit", surface.name)
			}
			if !errors.Is(writerErr, recoveryclaim.ErrBusy) {
				t.Fatalf("%s writer error = %v, want ErrBusy", surface.name, writerErr)
			}
			session, err := repoA.GetTaskSession(ctx, targetID)
			if err != nil {
				t.Fatalf("GetTaskSession: %v", err)
			}
			if session.State != models.TaskSessionStateWaitingForInput {
				t.Fatalf("target state = %s, want WAITING_FOR_INPUT", session.State)
			}
		})
	}
}

func TestPostgresSessionAdmissionWritersRereadAuthorityAfterSessionWait(t *testing.T) {
	for _, surface := range sessionAdmissionSurfaces() {
		t.Run(surface.name, func(t *testing.T) {
			repoA, repoB, observer := newTaskPostgresRepoPair(t)
			ctx := context.Background()
			taskID := "task-state-session-wait-" + surface.name
			environmentID := "environment-state-session-wait-" + surface.name
			holderID := "session-state-requester-" + surface.name
			targetID := "session-state-wait-target-" + surface.name
			seedTurnAdmissionRace(t, repoA, taskID, environmentID, holderID, targetID)

			sessionHolder, err := repoA.db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatalf("begin session-lock holder: %v", err)
			}
			defer func() { _ = sessionHolder.Rollback() }()
			if err := lockSessionTurnWrites(ctx, sessionHolder, repoA.db.DriverName(), targetID); err != nil {
				t.Fatalf("lock target session: %v", err)
			}

			writerPID := pgBackendPID(t, repoB.db)
			writerDone := make(chan struct{})
			var writerErr error
			go func() {
				writerErr = surface.mutate(ctx, repoB, targetID)
				close(writerDone)
			}()
			t.Cleanup(func() {
				_ = sessionHolder.Rollback()
				select {
				case <-writerDone:
				case <-time.After(5 * time.Second):
					t.Errorf("timed out waiting for %s writer during cleanup", surface.name)
				}
			})

			if err := waitForPostgresLock(ctx, observer, writerPID, writerDone); err != nil {
				t.Fatal(err)
			}
			if _, err := observer.ExecContext(ctx, observer.Rebind(`
				UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?
			`), "incarnation-state-replaced-"+surface.name, targetID); err != nil {
				t.Fatalf("replace target incarnation: %v", err)
			}
			if err := sessionHolder.Commit(); err != nil {
				t.Fatalf("commit session-lock holder: %v", err)
			}
			select {
			case <-writerDone:
			case <-time.After(5 * time.Second):
				t.Fatalf("timed out waiting for %s writer after session lock release", surface.name)
			}
			if writerErr == nil {
				t.Fatalf("%s writer succeeded after target incarnation changed", surface.name)
			}
			session, err := repoA.GetTaskSession(ctx, targetID)
			if err != nil {
				t.Fatalf("GetTaskSession: %v", err)
			}
			if session.State != models.TaskSessionStateWaitingForInput {
				t.Fatalf("target state = %s, want WAITING_FOR_INPUT", session.State)
			}
		})
	}
}

func TestPostgresSessionAdmissionWritersOrderBeforeRecoveryClaim(t *testing.T) {
	for _, surface := range sessionAdmissionSurfaces() {
		t.Run(surface.name, func(t *testing.T) {
			repoA, repoB, observer := newTaskPostgresRepoPair(t)
			ctx := context.Background()
			taskID := "task-state-writer-before-claim-" + surface.name
			environmentID := "environment-state-writer-before-claim-" + surface.name
			requesterID := "session-state-claim-requester-" + surface.name
			targetID := "session-state-writer-target-" + surface.name
			seedTurnAdmissionRace(t, repoA, taskID, environmentID, requesterID, targetID)
			claimRequest := recoveryClaimRequest(t, repoA, environmentID, taskID, requesterID, "operation-state-after-writer-"+surface.name, 1)

			sessionHolder, err := repoA.db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatalf("begin session-lock holder: %v", err)
			}
			defer func() { _ = sessionHolder.Rollback() }()
			if err := lockSessionTurnWrites(ctx, sessionHolder, repoA.db.DriverName(), targetID); err != nil {
				t.Fatalf("lock target session: %v", err)
			}

			writerPID := pgBackendPID(t, repoB.db)
			writerDone := make(chan struct{})
			var writerErr error
			go func() {
				writerErr = surface.mutate(ctx, repoB, targetID)
				close(writerDone)
			}()
			if err := waitForPostgresLock(ctx, observer, writerPID, writerDone); err != nil {
				t.Fatal(err)
			}

			var schema string
			if err := observer.GetContext(ctx, &schema, `SELECT current_schema()`); err != nil {
				t.Fatalf("read postgres test schema: %v", err)
			}
			claimDB := mustOpenTaskTestDB(t, testutil.PostgresDSNFromEnv(t), schema)
			claimPID := pgBackendPID(t, claimDB)
			claimDone := make(chan struct{})
			var claimErr error
			go func() {
				_, claimErr = recoveryclaim.Acquire(ctx, claimDB, claimRequest)
				close(claimDone)
			}()
			t.Cleanup(func() {
				_ = sessionHolder.Rollback()
				for name, done := range map[string]<-chan struct{}{"writer": writerDone, "claimant": claimDone} {
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Errorf("timed out waiting for %s during cleanup", name)
					}
				}
			})
			if err := waitForPostgresLock(ctx, observer, claimPID, claimDone); err != nil {
				t.Fatal(err)
			}
			select {
			case <-writerDone:
				t.Fatalf("%s writer returned while target session lock was held", surface.name)
			default:
			}
			select {
			case <-claimDone:
				t.Fatalf("recovery claimant returned while %s writer held the task lock", surface.name)
			default:
			}

			if err := sessionHolder.Commit(); err != nil {
				t.Fatalf("commit session-lock holder: %v", err)
			}
			select {
			case <-writerDone:
			case <-time.After(5 * time.Second):
				t.Fatalf("timed out waiting for %s writer", surface.name)
			}
			if writerErr != nil {
				t.Fatalf("%s writer: %v", surface.name, writerErr)
			}
			select {
			case <-claimDone:
			case <-time.After(5 * time.Second):
				t.Fatalf("timed out waiting for recovery claimant after %s writer", surface.name)
			}
			if !errors.Is(claimErr, recoveryclaim.ErrBusy) {
				t.Fatalf("recovery claim error = %v, want ErrBusy", claimErr)
			}
			var claims int
			if err := observer.GetContext(ctx, &claims, observer.Rebind(`
				SELECT count(*) FROM task_environment_recovery_claims WHERE task_environment_id = ?
			`), environmentID); err != nil {
				t.Fatalf("count recovery claims: %v", err)
			}
			if claims != 0 {
				t.Fatalf("recovery claim count = %d, want 0", claims)
			}
		})
	}
}
