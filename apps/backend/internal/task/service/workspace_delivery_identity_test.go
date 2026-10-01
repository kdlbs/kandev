package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/stretchr/testify/require"
)

type workspaceGenerationFailure struct{ repository.SessionRepository }

func (workspaceGenerationFailure) GetCurrentHarnessSessionGeneration(context.Context, string, string) (*models.HarnessSessionGeneration, error) {
	return nil, errors.New("generation storage unavailable")
}

func TestWorkspaceDeliveryIdentityRetainsCurrentGeneration(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	session := &models.TaskSession{ID: "workspace-generation", TaskID: "task-123", QueueIncarnationID: "persisted-owner", State: models.TaskSessionStateCompleted}
	require.NoError(t, repo.CreateTaskSession(ctx, session))
	committed, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 3, NativeSessionID: "native-session", CreationReason: "context_continuation",
	}, 0)
	require.NoError(t, err)
	require.True(t, committed)
	info, err := svc.GetWorkspaceInfoForSession(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.Equal(t, "persisted-owner", info.DeliveryIncarnationID)
	require.Equal(t, uint64(3), info.DeliveryHarnessGeneration)
	require.Equal(t, "persisted-owner:g3", info.DeliveryStreamID)
}

func TestWorkspaceDeliveryIdentityFailsClosedOnGenerationReadError(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.sessions = workspaceGenerationFailure{SessionRepository: repo}
	incarnationID, generation, err := svc.workspaceDeliveryIdentity(context.Background(), &models.TaskSession{ID: "session", QueueIncarnationID: "incarnation"})
	require.ErrorContains(t, err, "load workspace delivery generation")
	require.Empty(t, incarnationID)
	require.Zero(t, generation)
}
