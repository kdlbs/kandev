package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
	"testing"
)

type nilConfigChatSessionStore struct{ sessionExecutorStore }

func (r nilConfigChatSessionStore) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return nil, nil
}

func TestConfigChatRestartNilSessionReturnsNotFound(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "old-task", "old-session", "step1")
	manager := &mockAgentManager{}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), manager)
	svc.repo = nilConfigChatSessionStore{svc.repo}
	deleted := false
	err := svc.RetireConfigChatSession(context.Background(), "old-task", "old-session", func(context.Context) error { deleted = true; return nil })
	assert.ErrorIs(t, err, models.ErrTaskSessionNotFound)
	assert.False(t, deleted)
	assert.Empty(t, manager.stopAgentWithReasonArgs)
}
