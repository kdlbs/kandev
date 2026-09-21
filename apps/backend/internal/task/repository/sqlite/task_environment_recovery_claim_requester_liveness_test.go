package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

func TestTaskEnvironmentRecoveryClaimLiveRequesterBlocks(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(t *testing.T, repo *Repository, sessionID, taskID string)
		wantErr error
	}{
		{name: "pre-launch", wantErr: nil},
		{
			name: "open-turn",
			prepare: func(t *testing.T, repo *Repository, sessionID, taskID string) {
				t.Helper()
				if err := repo.CreateTurn(t.Context(), &models.Turn{
					ID: "turn-live-requester", TaskSessionID: sessionID, TaskID: taskID,
				}); err != nil {
					t.Fatalf("CreateTurn: %v", err)
				}
			},
			wantErr: recoveryclaim.ErrBusy,
		},
		{
			name: "running-executor",
			prepare: func(t *testing.T, repo *Repository, sessionID, taskID string) {
				t.Helper()
				if err := repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{
					ID: sessionID, SessionID: sessionID, TaskID: taskID,
					ExecutorID: "executor-live-requester", Status: models.ExecutorRunningStatusRunning,
				}); err != nil {
					t.Fatalf("UpsertExecutorRunning: %v", err)
				}
			},
			wantErr: recoveryclaim.ErrBusy,
		},
		{
			name: "running-state",
			prepare: func(t *testing.T, repo *Repository, sessionID, _ string) {
				t.Helper()
				if err := repo.UpdateTaskSessionState(t.Context(), sessionID, models.TaskSessionStateRunning, ""); err != nil {
					t.Fatalf("UpdateTaskSessionState: %v", err)
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
			if test.prepare != nil {
				test.prepare(t, repo, sessionID, taskID)
			}

			claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(context.Background(),
				recoveryClaimRequest(t, repo, environmentID, taskID, sessionID, "operation-live-requester", 1))
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("AcquireTaskEnvironmentRecoveryClaim error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && claim == nil {
				t.Fatal("AcquireTaskEnvironmentRecoveryClaim returned no claim")
			}
			if claim != nil {
				t.Cleanup(func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(t.Context(), claim) })
			}
		})
	}
}
