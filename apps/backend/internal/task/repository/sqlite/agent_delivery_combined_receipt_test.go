package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

type combinedReceiptProjector interface {
	ReceiveAndProjectCanonicalAgentDeliveryEventsWithReceipt(context.Context, []*models.AgentDeliveryEvent, []*models.AgentDeliveryEffect, int64) ([]models.CanonicalAgentDeliveryProjectionResult, *models.ConversationMutationReceipt, error)
}

func TestCombinedConversationReceipt(t *testing.T) {
	for _, database := range []string{"sqlite", "postgres"} {
		t.Run(database, func(t *testing.T) {
			for _, existing := range []bool{false, true} {
				name := "new-turn"
				if existing {
					name = "existing-turn"
				}
				t.Run(name, func(t *testing.T) {
					repo := canonicalReceiptRepo(t, database)
					if existing {
						seedCombinedDeliveryBatchSession(t, repo)
					} else {
						seedAgentDeliveryTestSession(t, repo, combinedDeliverySessionID, "incarnation-combined-batch")
					}
					projector, ok := any(repo).(combinedReceiptProjector)
					require.True(t, ok, "combined writer must return its committed conversation receipt")
					ctx := t.Context()
					before, err := repo.ReadConversationRevision(ctx, combinedDeliverySessionID)
					require.NoError(t, err)
					events := []*models.AgentDeliveryEvent{combinedDeliveryBatchEvent(t, 1, "a"), combinedDeliveryBatchEvent(t, 2, "b")}
					results, receipt, err := projector.ReceiveAndProjectCanonicalAgentDeliveryEventsWithReceipt(ctx, events, combinedDeliveryBatchEffects(events), 2)
					require.NoError(t, err)
					require.Len(t, results, 2)
					require.NotNil(t, receipt)
					require.True(t, receipt.Complete)
					require.Equal(t, before.Revision, receipt.BaseRevision)
					writes := int64(1)
					if !existing {
						writes = 2
					}
					require.Equal(t, before.Revision+writes, receipt.Revision)
					require.Len(t, receipt.Operations, int(writes))
					snapshot := receipt.Operations[len(receipt.Operations)-1].Message
					require.NotNil(t, snapshot)
					require.Equal(t, "ab", snapshot.Content)
					if !existing {
						require.Equal(t, snapshot.TurnID, receipt.Operations[0].Turn.ID)
					}
					next := combinedDeliveryBatchEvent(t, 3, "c")
					mixed := []*models.AgentDeliveryEvent{events[1], next}
					results, updated, err := projector.ReceiveAndProjectCanonicalAgentDeliveryEventsWithReceipt(ctx, mixed, combinedDeliveryBatchEffects(mixed), 3)
					require.NoError(t, err)
					require.True(t, results[0].AlreadyProjected)
					require.False(t, results[1].AlreadyProjected)
					require.NotNil(t, updated)
					require.True(t, updated.Complete)
					require.Equal(t, receipt.Revision, updated.BaseRevision)
					require.Equal(t, "abc", updated.Operations[0].Message.Content)
					require.Equal(t, "ab", snapshot.Content)
					_, replayed, err := projector.ReceiveAndProjectCanonicalAgentDeliveryEventsWithReceipt(ctx, mixed, combinedDeliveryBatchEffects(mixed), 3)
					require.NoError(t, err)
					require.Nil(t, replayed)
					empty, receipt, err := projector.ReceiveAndProjectCanonicalAgentDeliveryEventsWithReceipt(ctx, nil, nil, 3)
					require.NoError(t, err)
					require.Empty(t, empty)
					require.Nil(t, receipt)
				})
			}
		})
	}
}

func TestCombinedConversationReceiptRollback(t *testing.T) {
	for _, database := range []string{"sqlite", "postgres"} {
		t.Run(database, func(t *testing.T) {
			repo := canonicalReceiptRepo(t, database)
			seedCombinedDeliveryBatchSession(t, repo)
			projector, ok := any(repo).(combinedReceiptProjector)
			require.True(t, ok)
			ctx := t.Context()
			before, err := repo.ReadConversationRevision(ctx, combinedDeliverySessionID)
			require.NoError(t, err)
			events := []*models.AgentDeliveryEvent{combinedDeliveryBatchEvent(t, 1, "a"), combinedDeliveryBatchEvent(t, 2, "b")}
			effects := combinedDeliveryBatchEffects(events)
			effects[1].EffectKey = effects[0].EffectKey
			results, receipt, err := projector.ReceiveAndProjectCanonicalAgentDeliveryEventsWithReceipt(ctx, events, effects, 2)
			require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryEffectConflict)
			require.Nil(t, results)
			require.Nil(t, receipt)
			after, err := repo.ReadConversationRevision(ctx, combinedDeliverySessionID)
			require.NoError(t, err)
			require.Equal(t, before.Revision, after.Revision)
			require.Zero(t, countCombinedDeliveryRows(t, repo, `SELECT COUNT(*) FROM agent_delivery_inbox WHERE stream_id = ?`, combinedDeliveryStreamID))
			require.Zero(t, countCombinedDeliveryRows(t, repo, `SELECT COUNT(*) FROM task_session_messages WHERE id = ?`, combinedDeliveryMessageID))
			_, receipt, err = projector.ReceiveAndProjectCanonicalAgentDeliveryEventsWithReceipt(ctx, events, combinedDeliveryBatchEffects(events), 2)
			require.NoError(t, err)
			require.NotNil(t, receipt)
			require.True(t, receipt.Complete)
		})
	}
}
