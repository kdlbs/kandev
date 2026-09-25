package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPostgresMessageReplacementKeepsExistingBehavior(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	seedPostgresTask(t, repo, "retention-pg")
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "retention-session-pg", TaskID: "retention-pg"}))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "retention-turn-pg", TaskSessionID: "retention-session-pg", TaskID: "retention-pg"}))
	message := &models.Message{ID: "retention-message-pg", TaskID: "retention-pg", TaskSessionID: "retention-session-pg", TurnID: "retention-turn-pg", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolExecute, Metadata: map[string]any{"payload_retention": map[string]any{"version": 1}}}
	require.NoError(t, repo.CreateMessage(ctx, message))
	message.Metadata = map[string]any{"result": "replacement"}
	require.NoError(t, repo.UpdateMessage(ctx, message))
	stored, err := repo.GetMessage(ctx, message.ID)
	require.NoError(t, err)
	require.Equal(t, "replacement", stored.Metadata["result"])
	require.False(t, models.ToolPayloadRemoved(stored.Metadata))
}
