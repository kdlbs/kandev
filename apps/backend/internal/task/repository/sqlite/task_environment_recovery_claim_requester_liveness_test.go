package sqlite

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

func TestTaskEnvironmentRecoveryClaimLiveRequesterBlocks(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(t *testing.T, repo *Repository, sessionID, taskID, environmentID string)
		wantErr error
	}{
		{name: "pre-launch", wantErr: nil},
		{
			name: "requester-open-turn",
			prepare: func(t *testing.T, repo *Repository, sessionID, taskID, _ string) {
				t.Helper()
				now := time.Now().UTC()
				if _, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`
					INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?)
				`), "turn-live-requester", sessionID, taskID, now, now, now); err != nil {
					t.Fatalf("seed requester turn after claim: %v", err)
				}
			},
			wantErr: recoveryclaim.ErrBusy,
		},
		{
			name: "requester-running-executor",
			prepare: func(t *testing.T, repo *Repository, sessionID, taskID, _ string) {
				t.Helper()
				now := time.Now().UTC()
				if _, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`
					INSERT INTO executors_running (id, session_id, task_id, executor_id, status, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?, ?)
				`), sessionID, sessionID, taskID, "executor-live-requester", models.ExecutorRunningStatusRunning, now, now); err != nil {
					t.Fatalf("seed requester executor after claim: %v", err)
				}
			},
			wantErr: recoveryclaim.ErrBusy,
		},
		{
			name: "attached-consumer-open-turn",
			prepare: func(t *testing.T, repo *Repository, _ string, taskID, environmentID string) {
				t.Helper()
				const consumerID = "session-live-attached-consumer"
				now := time.Now().UTC()
				if _, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`
					INSERT INTO task_sessions (
						id, task_id, queue_incarnation_id, state, started_at, updated_at, task_environment_id
					) VALUES (?, ?, ?, ?, ?, ?, ?)
				`), consumerID, taskID, "incarnation-live-attached-consumer", models.TaskSessionStateWaitingForInput, now, now, environmentID); err != nil {
					t.Fatalf("seed attached consumer after claim: %v", err)
				}
				if _, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`
					INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?)
				`), "turn-live-attached-consumer", consumerID, taskID, now, now, now); err != nil {
					t.Fatalf("seed attached consumer turn after claim: %v", err)
				}
			},
			wantErr: recoveryclaim.ErrBusy,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			const (
				taskID        = "task-live-requester"
				environmentID = "environment-live-requester"
				sessionID     = "session-live-requester"
			)
			repo := newRepoForEntityTests(t)
			seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
			if err := repo.CreateTaskSession(t.Context(), &models.TaskSession{
				ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
				State: models.TaskSessionStateCreated,
			}); err != nil {
				t.Fatalf("CreateTaskSession: %v", err)
			}
			request := recoveryClaimRequest(t, repo, environmentID, taskID, sessionID, "operation-live-requester", 1)
			claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(context.Background(), request)
			if err != nil {
				t.Fatalf("initial AcquireTaskEnvironmentRecoveryClaim: %v", err)
			}
			t.Cleanup(func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(t.Context(), claim) })

			if test.prepare != nil {
				test.prepare(t, repo, sessionID, taskID, environmentID)
			}
			before := snapshotRecoveryClaim(t, repo, sessionID, environmentID)

			replayed, err := repo.AcquireTaskEnvironmentRecoveryClaim(context.Background(), request)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("replay AcquireTaskEnvironmentRecoveryClaim error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr != nil {
				if replayed != nil {
					t.Fatalf("replayed claim = %+v, want nil", replayed)
				}
			} else if !reflect.DeepEqual(replayed, claim) {
				t.Errorf("replayed claim = %+v, want unchanged %+v", replayed, claim)
			}
			assertRecoveryClaimSnapshot(t, snapshotRecoveryClaim(t, repo, sessionID, environmentID), before)
		})
	}
}
