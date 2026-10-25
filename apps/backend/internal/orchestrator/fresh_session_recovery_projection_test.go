package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// @covers AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.22
// @covers AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.23
func TestFreshSessionRecoveryStatePublication(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task", "session", "step")
	session, err := repo.GetTaskSession(ctx, "session")
	require.NoError(t, err)
	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb
	core, observed := observer.New(zap.WarnLevel)
	svc.logger, err = logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	svc.publishTaskSessionStateChanged(ctx, session.TaskID, session.ID,
		models.TaskSessionStateCreated, models.TaskSessionStateStarting, "", nil, session)
	require.Len(t, eb.events, 1)
	data := eb.events[0].event.Data.(map[string]interface{})
	blocks, ok := data["session_recovery_blocks"].([]dto.SessionRecoveryBlockDTO)
	require.True(t, ok)
	require.NotNil(t, blocks)
	require.Empty(t, blocks)
	require.Empty(t, observed.All())

	svc.repo = &freshProjectionFailureRepo{Repository: repo}
	svc.publishTaskSessionStateChanged(ctx, session.TaskID, session.ID,
		models.TaskSessionStateCreated, models.TaskSessionStateStarting, "", nil, session)
	require.Len(t, eb.events, 2)
	data = eb.events[1].event.Data.(map[string]interface{})
	require.NotContains(t, data, "session_recovery_blocks")
	entries := observed.FilterMessage("cannot project session recovery blocks").All()
	require.Len(t, entries, 1)
	require.Equal(t, "generation read failed", entries[0].ContextMap()["error"])
}

type freshProjectionFailureRepo struct {
	*taskrepo.Repository
}

func (*freshProjectionFailureRepo) GetCurrentHarnessSessionGeneration(
	context.Context, string, string,
) (*models.HarnessSessionGeneration, error) {
	return nil, errors.New("generation read failed")
}
