package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

type canonicalReceiptProjector interface {
	ProjectCanonicalAgentDeliveryEventsWithReceipt(context.Context, []*models.AgentDeliveryEvent, []*models.AgentDeliveryEffect) ([]models.CanonicalAgentDeliveryProjectionResult, *models.ConversationMutationReceipt, error)
}

func TestCanonicalConversationReceipt(t *testing.T) {
	for _, database := range []string{"sqlite", "postgres"} {
		t.Run(database, func(t *testing.T) {
			for _, existingTurn := range []bool{false, true} {
				name := "new-turn"
				if existingTurn {
					name = "existing-turn"
				}
				t.Run(name, func(t *testing.T) {
					testCanonicalConversationReceipt(t, canonicalReceiptRepo(t, database), existingTurn)
				})
			}
		})
	}
}

func canonicalReceiptRepo(t *testing.T, database string) *Repository {
	t.Helper()
	if database == "sqlite" {
		return newRepoForSessionTests(t)
	}
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	return repo
}

func testCanonicalConversationReceipt(t *testing.T, repo *Repository, existingTurn bool) {
	t.Helper()
	ctx := context.Background()
	seedAgentDeliveryTestSession(t, repo, "receipt-session", "receipt-incarnation")
	session, err := repo.GetTaskSession(ctx, "receipt-session")
	require.NoError(t, err)
	if existingTurn {
		require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "receipt-turn", TaskID: session.TaskID, TaskSessionID: session.ID}))
	}
	before, err := repo.ReadConversationRevision(ctx, session.ID)
	require.NoError(t, err)
	projector, ok := any(repo).(canonicalReceiptProjector)
	require.True(t, ok, "canonical persistence must return its committed conversation receipt")
	events := canonicalReceiptEvents(t, repo, 1, []string{"a", "b", "c"}, streams.EventTypeMessageChunk)
	appended, receipt, err := projector.ProjectCanonicalAgentDeliveryEventsWithReceipt(ctx, events, make([]*models.AgentDeliveryEffect, len(events)))
	require.NoError(t, err)
	require.Len(t, appended, 3)
	require.False(t, appended[0].Appended)
	require.True(t, appended[1].Appended)
	require.True(t, appended[2].Appended)
	require.NotNil(t, receipt)
	require.True(t, receipt.Complete)
	require.Equal(t, before.Revision, receipt.BaseRevision)
	writes := int64(2)
	if existingTurn {
		writes = 1
	}
	require.Equal(t, before.Revision+writes, receipt.Revision)
	require.Len(t, receipt.Operations, int(writes))
	message := receipt.Operations[len(receipt.Operations)-1].Message
	require.NotNil(t, message)
	require.Equal(t, "abc", message.Content)
	if !existingTurn {
		require.Equal(t, models.ConversationEntityTurn, receipt.Operations[0].Entity)
		require.Equal(t, message.TurnID, receipt.Operations[0].Turn.ID)
	}
	next := canonicalReceiptEvents(t, repo, 4, []string{"d"}, streams.EventTypeMessageChunk)
	_, updated, err := projector.ProjectCanonicalAgentDeliveryEventsWithReceipt(ctx, next, []*models.AgentDeliveryEffect{nil})
	require.NoError(t, err)
	require.True(t, updated.Complete)
	require.Equal(t, receipt.Revision, updated.BaseRevision)
	require.Equal(t, "abcd", updated.Operations[0].Message.Content)
	require.Equal(t, "abc", message.Content, "earlier receipt is an immutable transaction snapshot")
	_, replayed, err := projector.ProjectCanonicalAgentDeliveryEventsWithReceipt(ctx, next, []*models.AgentDeliveryEffect{nil})
	require.NoError(t, err)
	require.Nil(t, replayed, "an already projected event has no new mutation receipt")
}

func canonicalReceiptEvents(t *testing.T, repo *Repository, first int64, chunks []string, eventType string) []*models.AgentDeliveryEvent {
	t.Helper()
	events := make([]*models.AgentDeliveryEvent, 0, len(chunks))
	for i, chunk := range chunks {
		payload, err := json.Marshal(streams.AgentEvent{Type: eventType, Text: chunk, ReasoningText: chunk, CanonicalMessageID: "receipt-message"})
		require.NoError(t, err)
		event := &models.AgentDeliveryEvent{
			SessionID: "receipt-session", IncarnationID: "receipt-incarnation", HarnessGeneration: 1,
			StreamID: "receipt-stream", Sequence: first + int64(i), EventType: eventType, Payload: payload,
		}
		inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), event, event.Sequence)
		require.NoError(t, err)
		require.True(t, inserted)
		events = append(events, event)
	}
	return events
}

func TestCanonicalConversationReceiptRollback(t *testing.T) {
	for _, database := range []string{"sqlite", "postgres"} {
		t.Run(database, func(t *testing.T) {
			repo := canonicalReceiptRepo(t, database)
			seedAgentDeliveryTestSession(t, repo, "receipt-session", "receipt-incarnation")
			events := canonicalReceiptEvents(t, repo, 1, []string{"a", "b"}, streams.EventTypeMessageChunk)
			before, err := repo.ReadConversationRevision(t.Context(), "receipt-session")
			require.NoError(t, err)
			effects := []*models.AgentDeliveryEffect{
				{EffectKey: "conflict", EffectType: "test", StreamID: "receipt-stream", Sequence: 1},
				{EffectKey: "conflict", EffectType: "test", StreamID: "receipt-stream", Sequence: 2},
			}
			appended, receipt, err := requireCanonicalReceiptProjector(t, repo).ProjectCanonicalAgentDeliveryEventsWithReceipt(t.Context(), events, effects)
			require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryEffectConflict)
			require.Nil(t, receipt)
			require.Nil(t, appended)
			after, err := repo.ReadConversationRevision(t.Context(), "receipt-session")
			require.NoError(t, err)
			require.Equal(t, before.Revision, after.Revision)
			cursor, err := repo.GetAgentDeliveryCursor(t.Context(), "receipt-stream")
			require.NoError(t, err)
			require.Zero(t, cursor.ProjectedSequence)
			message, err := repo.GetMessage(t.Context(), "receipt-message")
			require.ErrorIs(t, err, sql.ErrNoRows)
			require.Nil(t, message)
		})
	}
}

func TestCanonicalConversationReceiptThinking(t *testing.T) {
	for _, database := range []string{"sqlite", "postgres"} {
		t.Run(database, func(t *testing.T) {
			repo := canonicalReceiptRepo(t, database)
			seedAgentDeliveryTestSession(t, repo, "receipt-session", "receipt-incarnation")
			events := canonicalReceiptEvents(t, repo, 1, []string{"first", " second"}, streams.EventTypeReasoning)
			_, receipt, err := requireCanonicalReceiptProjector(t, repo).ProjectCanonicalAgentDeliveryEventsWithReceipt(t.Context(), events, []*models.AgentDeliveryEffect{nil, nil})
			require.NoError(t, err)
			require.True(t, receipt.Complete)
			message := receipt.Operations[1].Message
			require.Equal(t, models.MessageTypeThinking, message.Type)
			require.Empty(t, message.Content)
			require.Equal(t, "first second", message.Metadata["thinking"])
			next := canonicalReceiptEvents(t, repo, 3, []string{" third"}, streams.EventTypeReasoning)
			_, updated, err := requireCanonicalReceiptProjector(t, repo).ProjectCanonicalAgentDeliveryEventsWithReceipt(t.Context(), next, []*models.AgentDeliveryEffect{nil})
			require.NoError(t, err)
			require.Equal(t, receipt.Revision, updated.BaseRevision)
			require.Equal(t, "first second third", updated.Operations[0].Message.Metadata["thinking"])
		})
	}
}

func requireCanonicalReceiptProjector(t *testing.T, repo *Repository) canonicalReceiptProjector {
	t.Helper()
	projector, ok := any(repo).(canonicalReceiptProjector)
	require.True(t, ok, "canonical persistence must return its committed conversation receipt")
	return projector
}
