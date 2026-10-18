package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestResumeSession_DockerReservationFailureRollsBackStartingState(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	repo.sessions["sess-1"].State = models.TaskSessionStateFailed
	repo.sessions["sess-1"].ExecutorID = models.ExecutorIDLocalDocker
	repo.executors[models.ExecutorIDLocalDocker] = &models.Executor{
		ID: models.ExecutorIDLocalDocker, Type: models.ExecutorTypeLocalDocker, Status: models.ExecutorStatusActive,
	}
	reservationErr := errors.New("environment reservation failed")
	repo.createTaskEnvironmentErr = reservationErr
	manager := &mockAgentManager{launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
		t.Fatal("launch called after reservation failed")
		return nil, nil
	}}
	executor := newTestExecutor(t, manager, repo)
	_, err := executor.ResumeSession(context.Background(), repo.sessions["sess-1"], true)
	if !errors.Is(err, reservationErr) {
		t.Fatalf("ResumeSession error = %v, want reservation error", err)
	}
	if current := repo.sessions["sess-1"]; current.State != models.TaskSessionStateFailed {
		t.Fatalf("session state = %s, want failed", current.State)
	}
}
