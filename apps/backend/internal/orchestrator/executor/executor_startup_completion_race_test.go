package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type completedDuringStartupReadRepository struct {
	*mockRepository
	completeAfterRead bool
}

func (r *completedDuringStartupReadRepository) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	session, err := r.mockRepository.GetTaskSession(ctx, id)
	if err == nil && session != nil && r.completeAfterRead {
		r.completeAfterRead = false
		if err := r.UpdateTaskSessionState(ctx, id, models.TaskSessionStateWaitingForInput, ""); err != nil {
			return nil, err
		}
	}
	return session, err
}

func TestProcessStartupDoesNotReviveTurnCompletedAfterStateRead(t *testing.T) {
	repo := &completedDuringStartupReadRepository{mockRepository: newMockRepository(), completeAfterRead: true}
	repo.sessions["session"] = &models.TaskSession{ID: "session", TaskID: "task", State: models.TaskSessionStateStarting}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	require.False(t, exec.markSessionRunningAfterProcessStart(context.Background(), "task", "session"))
	session, err := repo.GetTaskSession(context.Background(), "session")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
}
