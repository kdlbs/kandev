package storeconformance

import (
	"context"
	"github.com/kandev/kandev/internal/orchestration/models"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestAssistantAttentionPersistenceEngines(t *testing.T) {
	for _, engine := range []testconformance.EngineName{testconformance.EngineSQLite, testconformance.EnginePostgres} {
		t.Run(string(engine), func(t *testing.T) {
			repo, b := authorityStoreFixture(t, engine)
			ctx := context.Background()
			now := time.Now().UTC()
			sources := []models.AttentionSource{{SourceID: "question", SessionID: "older", Kind: "question", State: "pending", SourceRevision: "source-v1", Summary: "Choose a sample color"}}
			changed, err := repo.ProjectAttention(ctx, b, b.ConversationID, sources, now)
			require.NoError(t, err)
			require.True(t, changed)
			changed, err = repo.ProjectAttention(ctx, b, b.ConversationID, sources, now)
			require.NoError(t, err)
			require.False(t, changed)
			rows, err := repo.AttentionPage(ctx, b.ID, "", 1)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			wakes, err := repo.PendingAttentionWakes(ctx, b.ID, b.ConversationID)
			require.NoError(t, err)
			require.Len(t, wakes, 1)
			require.NoError(t, repo.Migrate())
			require.NoError(t, repo.AcknowledgeAttentionWake(ctx, wakes[0]))
			sources[0].State = "unknown"
			_, err = repo.ProjectAttention(ctx, b, b.ConversationID, sources, now)
			require.NoError(t, err)
			sources[0].State = "pending"
			_, err = repo.ProjectAttention(ctx, b, b.ConversationID, sources, now)
			require.NoError(t, err)
			wakes, err = repo.PendingAttentionWakes(ctx, b.ID, b.ConversationID)
			require.NoError(t, err)
			require.Empty(t, wakes)
			sources[0].SourceRevision = "source-v2"
			_, err = repo.ProjectAttention(ctx, b, b.ConversationID, sources, now)
			require.NoError(t, err)
			wakes, err = repo.PendingAttentionWakes(ctx, b.ID, b.ConversationID)
			require.NoError(t, err)
			require.Len(t, wakes, 1)
			foreign, err := repo.AttentionPage(ctx, "foreign", "", 100)
			require.NoError(t, err)
			require.Empty(t, foreign)
			b.Version++
			_, err = repo.ProjectAttention(ctx, b, b.ConversationID, sources, now)
			require.ErrorIs(t, err, models.ErrConflict)
		})
	}
}
