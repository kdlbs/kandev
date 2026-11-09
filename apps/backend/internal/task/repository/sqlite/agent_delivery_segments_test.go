package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestCanonicalDeliveryTextSegmentsSplitAtTopLevelToolStarts(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedAgentDeliveryTestSession(t, repo, "segment-session", "segment-incarnation")

	events := []*models.AgentDeliveryEvent{
		canonicalTextSegmentEvent(t, 1, "before "),
		canonicalTextSegmentEvent(t, 2, "tool"),
		canonicalToolBoundaryEvent(t, 3, "", streams.EventTypeToolCall),
		canonicalTextSegmentEvent(t, 4, "after "),
		canonicalTextSegmentEvent(t, 5, "tool"),
	}
	for _, event := range events {
		_, err := repo.ReceiveAgentDeliveryEvent(ctx, event, event.Sequence)
		require.NoError(t, err)
		if event.EventType == streams.EventTypeToolCall {
			_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
		} else {
			_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, nil)
		}
		require.NoError(t, err)
	}

	var messages []struct {
		ID      string `db:"id"`
		Content string `db:"content"`
	}
	require.NoError(t, repo.db.SelectContext(ctx, &messages, `
		SELECT id, content FROM task_session_messages
		WHERE task_session_id = ? AND author_type = ? ORDER BY id`,
		"segment-session", string(models.MessageAuthorAgent)))
	require.Len(t, messages, 2, "a top-level tool call ends the prior text segment")
	require.ElementsMatch(t, []string{"before tool", "after tool"}, []string{messages[0].Content, messages[1].Content})
	require.NotEqual(t, messages[0].ID, messages[1].ID)
}

func TestCanonicalDeliveryTextSegmentsIgnoreNestedToolsAndUpdates(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedAgentDeliveryTestSession(t, repo, "nested-segment-session", "nested-segment-incarnation")
	events := []*models.AgentDeliveryEvent{
		canonicalTextSegmentEventFor(t, "nested-segment-session", "nested-segment-incarnation", "nested-segment-stream", 1, "before "),
		canonicalSegmentEventFor(t, "nested-segment-session", "nested-segment-incarnation", "nested-segment-stream", 2,
			streams.AgentEvent{Type: streams.EventTypeToolCall, ToolCallID: "nested", ParentToolCallID: "parent"}),
		canonicalSegmentEventFor(t, "nested-segment-session", "nested-segment-incarnation", "nested-segment-stream", 3,
			streams.AgentEvent{Type: streams.EventTypeToolUpdate, ToolCallID: "nested", ParentToolCallID: "parent"}),
		canonicalTextSegmentEventFor(t, "nested-segment-session", "nested-segment-incarnation", "nested-segment-stream", 4, "after"),
	}
	for _, event := range events {
		_, err := repo.ReceiveAgentDeliveryEvent(ctx, event, event.Sequence)
		require.NoError(t, err)
		if event.EventType == streams.EventTypeMessageChunk {
			_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, nil)
		} else {
			_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
		}
		require.NoError(t, err)
	}

	var messages []struct {
		ID      string `db:"id"`
		Content string `db:"content"`
	}
	require.NoError(t, repo.db.SelectContext(ctx, &messages, `
		SELECT id, content FROM task_session_messages
		WHERE task_session_id = ? AND author_type = ?`,
		"nested-segment-session", string(models.MessageAuthorAgent)))
	require.Len(t, messages, 1)
	require.Equal(t, "before after", messages[0].Content)
}

func TestCanonicalDeliveryTextSegmentsPreserveNativeAndReasoningIdentity(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedAgentDeliveryTestSession(t, repo, "native-segment-session", "native-segment-incarnation")
	nativeText := func(sequence int64, content string) *models.AgentDeliveryEvent {
		return canonicalSegmentEventFor(t, "native-segment-session", "native-segment-incarnation", "native-segment-stream", sequence,
			streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: content, ProtocolMessageID: "native-message", CanonicalMessageID: "native-message"})
	}
	reasoning := func(sequence int64, content string) *models.AgentDeliveryEvent {
		return canonicalSegmentEventFor(t, "native-segment-session", "native-segment-incarnation", "native-segment-stream", sequence,
			streams.AgentEvent{Type: streams.EventTypeReasoning, ReasoningText: content, CanonicalMessageID: "reasoning-message"})
	}
	events := []*models.AgentDeliveryEvent{
		nativeText(1, "Native before "),
		reasoning(2, "Thinking before "),
		canonicalToolBoundaryEventFor(t, "native-segment-session", "native-segment-incarnation", "native-segment-stream", 3),
		nativeText(4, "after"),
		reasoning(5, "after"),
	}
	for _, event := range events {
		_, err := repo.ReceiveAgentDeliveryEvent(ctx, event, event.Sequence)
		require.NoError(t, err)
		_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, nil)
		require.NoError(t, err)
	}

	var messages []struct {
		ID       string `db:"id"`
		Content  string `db:"content"`
		Type     string `db:"type"`
		Metadata string `db:"metadata"`
	}
	require.NoError(t, repo.db.SelectContext(ctx, &messages,
		"SELECT id, content, type, metadata FROM task_session_messages WHERE task_session_id = ? AND author_type = ?",
		"native-segment-session", string(models.MessageAuthorAgent)))
	var nativeTextMessage, thinkingMessage *struct {
		ID       string `db:"id"`
		Content  string `db:"content"`
		Type     string `db:"type"`
		Metadata string `db:"metadata"`
	}
	for i := range messages {
		message := &messages[i]
		switch message.Type {
		case string(models.MessageTypeMessage):
			nativeTextMessage = message
		case string(models.MessageTypeThinking):
			thinkingMessage = message
		}
	}
	require.NotNil(t, nativeTextMessage)
	require.Equal(t, "native-message", nativeTextMessage.ID)
	require.Equal(t, "Native before after", nativeTextMessage.Content)
	require.NotNil(t, thinkingMessage)
	require.NotEqual(t, nativeTextMessage.ID, thinkingMessage.ID)
	var thinkingMetadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(thinkingMessage.Metadata), &thinkingMetadata))
	require.Equal(t, "Thinking before after", thinkingMetadata["thinking"])
}

func TestCanonicalDeliveryTextSegmentsSplitAtCanonicalShellStart(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedAgentDeliveryTestSession(t, repo, "shell-segment-session", "shell-segment-incarnation")
	shellStart := canonicalSegmentEventFor(t, "shell-segment-session", "shell-segment-incarnation", "shell-segment-stream", 2,
		streams.AgentEvent{Type: streams.EventTypeToolCall, ToolCallID: "shell", ToolStatus: "pending", ToolTitle: "run",
			CanonicalMessageID: "shell-start-message",
			NormalizedPayload:  streams.NewShellExec("run", "/tmp", "", 0, false)})
	events := []*models.AgentDeliveryEvent{
		canonicalTextSegmentEventFor(t, "shell-segment-session", "shell-segment-incarnation", "shell-segment-stream", 1, "before"),
		shellStart,
		canonicalTextSegmentEventFor(t, "shell-segment-session", "shell-segment-incarnation", "shell-segment-stream", 3, "after"),
	}
	for _, event := range events {
		_, err := repo.ReceiveAgentDeliveryEvent(ctx, event, event.Sequence)
		require.NoError(t, err)
		_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, nil)
		require.NoError(t, err)
	}
	assertAgentTextSegmentRows(t, repo, "shell-segment-session", []string{"before", "after"})
	require.Equal(t, "shell-start-message", storedCanonicalMessageID(t, repo, shellStart.StreamID, shellStart.Sequence))
	_, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_inbox SET payload = ?, payload_pruned_at = ? WHERE stream_id = ? AND sequence = ?`),
		[]byte{}, repo.nowUTC(), shellStart.StreamID, shellStart.Sequence)
	require.NoError(t, err)
	duplicate, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, shellStart, nil)
	require.NoError(t, err)
	require.Equal(t, "shell-start-message", duplicate.CanonicalMessageID,
		"the canonical shell-start route returns its stored identity after payload pruning")
}

func TestCanonicalDeliveryTextSegmentsRespectMixedBatchOrder(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedAgentDeliveryTestSession(t, repo, "batch-segment-session", "batch-segment-incarnation")
	events := []*models.AgentDeliveryEvent{
		canonicalTextSegmentEventFor(t, "batch-segment-session", "batch-segment-incarnation", "batch-segment-stream", 1, "before"),
		canonicalSegmentEventFor(t, "batch-segment-session", "batch-segment-incarnation", "batch-segment-stream", 2,
			streams.AgentEvent{Type: streams.EventTypeToolCall, ToolCallID: "tool", ParentToolCallID: ""}),
		canonicalTextSegmentEventFor(t, "batch-segment-session", "batch-segment-incarnation", "batch-segment-stream", 3, "after"),
	}
	for _, event := range events {
		_, err := repo.ReceiveAgentDeliveryEvent(ctx, event, event.Sequence)
		require.NoError(t, err)
	}
	_, err := repo.ProjectCanonicalAgentDeliveryEvents(ctx, events, make([]*models.AgentDeliveryEffect, len(events)))
	require.NoError(t, err)
	assertAgentTextSegmentRows(t, repo, "batch-segment-session", []string{"before", "after"})
}

func TestCanonicalDeliveryTextSegmentRollbackRetryAndDuplicate(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedAgentDeliveryTestSession(t, repo, "retry-segment-session", "retry-segment-incarnation")
	tool := canonicalToolBoundaryEventFor(t, "retry-segment-session", "retry-segment-incarnation", "retry-segment-stream", 1)
	_, err := repo.ReceiveAgentDeliveryEvent(ctx, tool, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, tool, nil)
	require.NoError(t, err)

	textEvent := canonicalTextSegmentEventFor(t, "retry-segment-session", "retry-segment-incarnation", "retry-segment-stream", 2, "retry")
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, textEvent, 2)
	require.NoError(t, err)
	originalPayload := append([]byte(nil), textEvent.Payload...)
	_, err = repo.db.Exec("CREATE TRIGGER fail_segment_projection BEFORE UPDATE OF projected_at ON agent_delivery_inbox WHEN OLD.sequence = 2 BEGIN SELECT RAISE(ABORT,'injected segment projection failure'); END")
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, textEvent, nil)
	require.Error(t, err)
	require.Equal(t, originalPayload, textEvent.Payload, "projection failure must not leak a tentative ID into the caller event")
	require.Empty(t, textEvent.CanonicalMessageID, "projection failure must not leak a tentative ID into the caller event")
	var storedPayload []byte
	require.NoError(t, repo.db.Get(&storedPayload, "SELECT payload FROM agent_delivery_inbox WHERE stream_id = 'retry-segment-stream' AND sequence = 2"))
	require.Equal(t, originalPayload, storedPayload)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, "retry-segment-stream")
	require.NoError(t, err)
	require.EqualValues(t, 1, cursor.ProjectedSequence)
	requireSegmentBoundary(t, repo.db, "retry-segment-stream", 1)
	var failedMessageCount int
	require.NoError(t, repo.db.Get(&failedMessageCount, repo.db.Rebind(`
		SELECT COUNT(*) FROM task_session_messages
		WHERE task_session_id = ? AND author_type = ? AND content = ?`),
		"retry-segment-session", string(models.MessageAuthorAgent), "retry"))
	require.Zero(t, failedMessageCount, "failed projection must roll back the canonical message effect")
	_, err = repo.db.Exec("DROP TRIGGER fail_segment_projection")
	require.NoError(t, err)

	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, textEvent, nil)
	require.NoError(t, err)
	firstCommittedID := storedCanonicalMessageID(t, repo, textEvent.StreamID, textEvent.Sequence)
	require.NotEqual(t, "stable-text-base", firstCommittedID)
	var committedContent string
	require.NoError(t, repo.db.Get(&committedContent, repo.db.Rebind(`
		SELECT content FROM task_session_messages WHERE id = ?`), firstCommittedID))
	require.Equal(t, "retry", committedContent)
	prunedAt := repo.nowUTC()
	_, err = repo.db.Exec(repo.db.Rebind(`
		UPDATE agent_delivery_inbox SET payload = ?, payload_pruned_at = ?
		WHERE stream_id = ? AND sequence = ?`), []byte{}, prunedAt, textEvent.StreamID, textEvent.Sequence)
	require.NoError(t, err, "the committed ID remains available after payload pruning")

	laterTool := canonicalToolBoundaryEventFor(t, "retry-segment-session", "retry-segment-incarnation", "retry-segment-stream", 3)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, laterTool, 3)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, laterTool, nil)
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, textEvent, nil)
	require.NoError(t, err)
	require.Equal(t, firstCommittedID, storedCanonicalMessageID(t, repo, textEvent.StreamID, textEvent.Sequence),
		"a duplicate after a later boundary must retain the original committed ID")
	require.NoError(t, repo.db.Get(&committedContent, repo.db.Rebind(`
		SELECT content FROM task_session_messages WHERE id = ?`), firstCommittedID))
	require.Equal(t, "retry", committedContent, "a duplicate must not repeat the message effect")
}

func TestCanonicalDeliveryRetentionPruneRestartAndDuplicatePreserveCommittedID(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "delivery-segment-retention.db")
	var database *sqlx.DB
	openDatabase := func() *sqlx.DB {
		raw, err := internaldb.OpenSQLite(databasePath)
		require.NoError(t, err)
		database = sqlx.NewDb(raw, "sqlite3")
		return database
	}
	t.Cleanup(func() {
		if database != nil {
			_ = database.Close()
		}
	})

	database = openDatabase()
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	ctx := context.Background()
	seedAgentDeliveryTestSession(t, repo, "retained-segment-session", "retained-segment-incarnation")
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	repo.clockNow = func() time.Time { return now }
	submission := &models.AgentDeliverySubmission{
		ID: "prompt:retained-segment", SessionID: "retained-segment-session",
		IncarnationID: "retained-segment-incarnation", HarnessGeneration: 1,
		OwnerGeneration: 1, PayloadHash: "retained-segment-prompt-hash",
		Payload: []byte("protected prompt"), State: models.DeliverySubmissionDispatching,
	}
	_, err = repo.PrepareAgentDeliverySubmission(ctx, submission)
	require.NoError(t, err)
	textEvent := canonicalTextSegmentEventFor(t, submission.SessionID, submission.IncarnationID, "retained-segment-stream", 1, "retained text")
	textEvent.SubmissionID = submission.ID
	textEvent.ReceivedAt = now
	inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, textEvent, 1)
	require.NoError(t, err)
	require.True(t, inserted)
	projection, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, textEvent, completedRetentionEventEffect(textEvent))
	require.NoError(t, err)
	committedID := projection.CanonicalMessageID
	require.NotEmpty(t, committedID)

	now = now.Add(2 * time.Hour)
	retention, err := repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, retention.EventPayloadsPruned)
	require.Zero(t, retention.EventRowsRetired, "the dispatching submission keeps the receipt available for replay")
	var compacted struct {
		Payload       []byte     `db:"payload"`
		PayloadPruned *time.Time `db:"payload_pruned_at"`
		CanonicalID   string     `db:"canonical_message_id"`
	}
	require.NoError(t, database.Get(&compacted, database.Rebind(`
		SELECT payload, payload_pruned_at, canonical_message_id FROM agent_delivery_inbox
		WHERE stream_id = ? AND sequence = ?`), textEvent.StreamID, textEvent.Sequence))
	require.Empty(t, compacted.Payload)
	require.NotNil(t, compacted.PayloadPruned)
	require.Equal(t, committedID, compacted.CanonicalID, "payload compaction preserves the authoritative committed ID")

	require.NoError(t, database.Close(), "simulate backend shutdown after retention compaction")
	database = openDatabase()
	repo, err = NewWithDB(database, database, nil)
	require.NoError(t, err, "reopen after payload compaction")
	laterTool := canonicalToolBoundaryEventFor(t, submission.SessionID, submission.IncarnationID, textEvent.StreamID, 2)
	laterTool.ReceivedAt = now
	inserted, err = repo.ReceiveAgentDeliveryEvent(ctx, laterTool, 2)
	require.NoError(t, err)
	require.True(t, inserted)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, laterTool, nil)
	require.NoError(t, err)
	requireSegmentBoundary(t, database, textEvent.StreamID, 2)

	var reconstructed streams.AgentEvent
	require.NoError(t, json.Unmarshal(textEvent.Payload, &reconstructed))
	reconstructed.CanonicalMessageBaseID = "reconstructed-base-must-be-ignored"
	reconstructed.CanonicalMessageID = "reconstructed-id-must-be-ignored"
	duplicatePayload, err := json.Marshal(reconstructed)
	require.NoError(t, err)
	duplicate := *textEvent
	duplicate.Payload = duplicatePayload
	duplicate.CanonicalMessageID = "model-field-must-not-replace-stored-id"
	inserted, err = repo.ReceiveAgentDeliveryEvent(ctx, &duplicate, 2)
	require.NoError(t, err)
	require.False(t, inserted, "duplicate admission must preserve the projected receipt")
	duplicateProjection, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, &duplicate, nil)
	require.NoError(t, err)
	require.Equal(t, committedID, duplicateProjection.CanonicalMessageID,
		"a duplicate after reopen and a later boundary returns the exact stored ID")
	require.Equal(t, committedID, storedCanonicalMessageID(t, repo, textEvent.StreamID, textEvent.Sequence),
		"duplicate admission and projection cannot replace the authoritative ID")
	var content string
	require.NoError(t, database.Get(&content, database.Rebind(`SELECT content FROM task_session_messages WHERE id = ?`), committedID))
	require.Equal(t, "retained text", content, "replay must not repeat the message effect")
}

func TestCanonicalDeliveryPreUpgradePrunedReceiptDoesNotInventIdentity(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			database := openDeliveryRetentionTestDB(t, engine)
			seedPreRetentionDeliverySchema(t, database)
			repo, err := NewWithDB(database, database, nil)
			require.NoError(t, err, "upgrade the old receipt schema")
			const sessionID = "legacy-pruned-session"
			const incarnationID = "legacy-pruned-incarnation"
			const streamID = "legacy-pruned-stream"
			seedAgentDeliveryTestSession(t, repo, sessionID, incarnationID)
			now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
			event := canonicalTextSegmentEventFor(t, sessionID, incarnationID, streamID, 1, "previously projected text")
			event.ReceivedAt = now
			inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
			require.NoError(t, err)
			require.True(t, inserted)
			_, err = database.Exec(database.Rebind(`
				UPDATE agent_delivery_inbox
				SET payload = ?, projected_at = ?, payload_pruned_at = ?, canonical_message_id = ''
				WHERE stream_id = ? AND sequence = ?`), []byte{}, now, now, event.StreamID, event.Sequence)
			require.NoError(t, err)
			_, err = database.Exec(database.Rebind(`
				UPDATE agent_delivery_cursors SET projected_sequence = ? WHERE stream_id = ?`), event.Sequence, event.StreamID)
			require.NoError(t, err)

			tool := canonicalToolBoundaryEventFor(t, sessionID, incarnationID, streamID, 2)
			tool.ReceivedAt = now
			inserted, err = repo.ReceiveAgentDeliveryEvent(t.Context(), tool, 2)
			require.NoError(t, err)
			require.True(t, inserted)
			_, err = repo.ProjectAgentDeliveryEvent(t.Context(), tool, nil)
			require.NoError(t, err)
			requireSegmentBoundary(t, database, streamID, 2)

			var reconstructed streams.AgentEvent
			require.NoError(t, json.Unmarshal(event.Payload, &reconstructed))
			reconstructed.CanonicalMessageBaseID = "incoming-base-must-not-be-used"
			reconstructed.CanonicalMessageID = "incoming-id-must-not-be-used"
			duplicatePayload, err := json.Marshal(reconstructed)
			require.NoError(t, err)
			duplicate := *event
			duplicate.Payload = duplicatePayload
			duplicate.CanonicalMessageID = "incoming-model-id-must-not-be-used"
			inserted, err = repo.ReceiveAgentDeliveryEvent(t.Context(), &duplicate, 1)
			require.NoError(t, err)
			require.False(t, inserted)
			result, err := repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), &duplicate, nil)
			require.NoError(t, err)
			require.Empty(t, result.CanonicalMessageID,
				"an old pruned receipt with no persisted ID cannot derive one from incoming metadata or a later boundary")
			var retained struct {
				Payload            []byte     `db:"payload"`
				PayloadPrunedAt    *time.Time `db:"payload_pruned_at"`
				CanonicalMessageID string     `db:"canonical_message_id"`
				ProjectedAt        *time.Time `db:"projected_at"`
			}
			require.NoError(t, database.Get(&retained, database.Rebind(`
				SELECT payload, payload_pruned_at, canonical_message_id, projected_at FROM agent_delivery_inbox
				WHERE stream_id = ? AND sequence = ?`), event.StreamID, event.Sequence))
			require.Empty(t, retained.Payload)
			require.NotNil(t, retained.PayloadPrunedAt)
			require.NotNil(t, retained.ProjectedAt)
			require.Empty(t, retained.CanonicalMessageID)
			var messageCount int
			require.NoError(t, database.Get(&messageCount, database.Rebind(`
				SELECT COUNT(*) FROM task_session_messages WHERE task_session_id = ? AND author_type = ?`),
				event.SessionID, string(models.MessageAuthorAgent)))
			require.Zero(t, messageCount, "fallback must not create or notify a transcript row whose ID was not retained")
			cursor, err := repo.GetAgentDeliveryCursor(t.Context(), event.StreamID)
			require.NoError(t, err)
			require.EqualValues(t, 2, cursor.ProjectedSequence, "duplicate projection leaves the later committed cursor unchanged")

			positiveControl := canonicalTextSegmentEventFor(t, sessionID, incarnationID, streamID, 3, "newly committed output")
			positiveControl.ReceivedAt = now
			inserted, err = repo.ReceiveAgentDeliveryEvent(t.Context(), positiveControl, 3)
			require.NoError(t, err)
			require.True(t, inserted)
			positiveProjection, err := repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), positiveControl, nil)
			require.NoError(t, err)
			require.NotEmpty(t, positiveProjection.CanonicalMessageID)
			require.NoError(t, database.Get(&messageCount, database.Rebind(`
				SELECT COUNT(*) FROM task_session_messages WHERE task_session_id = ? AND author_type = ?`),
				sessionID, string(models.MessageAuthorAgent)))
			require.Equal(t, 1, messageCount, "a fresh committed event still creates its transcript row")
		})
	}
}

func TestCanonicalDeliveryTextSegmentBoundarySurvivesDatabaseReopen(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "delivery-segments.db")
	openDatabase := func() *sqlx.DB {
		raw, err := internaldb.OpenSQLite(databasePath)
		require.NoError(t, err)
		database := sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { _ = database.Close() })
		return database
	}

	database := openDatabase()
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	ctx := context.Background()
	seedAgentDeliveryTestSession(t, repo, "restart-segment-session", "restart-segment-incarnation")
	tool := canonicalToolBoundaryEventFor(t, "restart-segment-session", "restart-segment-incarnation", "restart-segment-stream", 1)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, tool, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, tool, nil)
	require.NoError(t, err)

	first := canonicalTextSegmentEventFor(t, "restart-segment-session", "restart-segment-incarnation", "restart-segment-stream", 2, "before restart ")
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, first, 2)
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, first, nil)
	require.NoError(t, err)
	committedID := storedCanonicalMessageID(t, repo, first.StreamID, first.Sequence)
	require.NoError(t, database.Close(), "simulate backend database shutdown")

	database = openDatabase()
	repo, err = NewWithDB(database, database, nil)
	require.NoError(t, err, "restore the backend repository after restart")
	duplicate, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, first, nil)
	require.NoError(t, err)
	require.Equal(t, committedID, duplicate.CanonicalMessageID,
		"replay after restart returns the committed ID from the inbox")

	continued := canonicalTextSegmentEventFor(t, "restart-segment-session", "restart-segment-incarnation", "restart-segment-stream", 3, "after restart")
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, continued, 3)
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, continued, nil)
	require.NoError(t, err)
	require.Equal(t, committedID, storedCanonicalMessageID(t, repo, continued.StreamID, continued.Sequence),
		"the persisted segment boundary remains authoritative after restart")
	var content string
	require.NoError(t, database.Get(&content, database.Rebind("SELECT content FROM task_session_messages WHERE id = ?"), committedID))
	require.Equal(t, "before restart after restart", content)
}

func TestCanonicalDeliverySegmentBoundaryMigration(t *testing.T) {
	runCanonicalDeliverySegmentBoundaryMigrationTest(t, "SQLite")
}

func TestCanonicalDeliverySegmentBoundaryPostgres(t *testing.T) {
	runCanonicalDeliverySegmentBoundaryMigrationTest(t, "Postgres")
}

func runCanonicalDeliverySegmentBoundaryMigrationTest(t *testing.T, engine string) {
	t.Helper()
	database := openDeliveryRetentionTestDB(t, engine)
	seedPreRetentionDeliverySchema(t, database)
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err, "upgrade the pre-boundary schema")
	seedAgentDeliveryTestSession(t, repo, "segment-upgrade-session", "segment-upgrade-incarnation")
	ctx := context.Background()
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	legacyTurnID := "segment-upgrade-legacy-turn"
	legacyTaskID := "delivery-fixture-task-segment-upgrade-session"
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: legacyTurnID, TaskID: legacyTaskID, TaskSessionID: "segment-upgrade-session",
		StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}))
	legacyMessage := &models.Message{
		ID: "stable-text-base", TaskID: legacyTaskID, TaskSessionID: "segment-upgrade-session",
		TurnID: legacyTurnID, AuthorType: models.MessageAuthorAgent,
		Content: "already projected output", Type: models.MessageTypeMessage,
		Metadata: map[string]any{"durable_delivery": true}, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateMessage(ctx, legacyMessage))
	for _, streamID := range []string{"segment-upgrade-text-first", "segment-upgrade-tool-first"} {
		seedLegacySegmentCursor(t, database, streamID, now)
		require.NoError(t, assertNullSegmentBoundary(database, streamID), "legacy cursors start without a segment boundary")
	}

	var historicalBefore struct {
		ID        string    `db:"id"`
		Content   string    `db:"content"`
		Metadata  string    `db:"metadata"`
		UpdatedAt time.Time `db:"updated_at"`
	}
	require.NoError(t, database.Get(&historicalBefore, database.Rebind("SELECT id, content, metadata, updated_at FROM task_session_messages WHERE id = ?"), legacyMessage.ID))
	textEvent := canonicalTextSegmentEventFor(t, "segment-upgrade-session", "segment-upgrade-incarnation", "segment-upgrade-text-first", 6, "after upgrade")
	textEvent.HarnessGeneration = 3
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, textEvent, textEvent.Sequence)
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, textEvent, nil)
	require.NoError(t, err)
	requireSegmentBoundary(t, database, textEvent.StreamID, 5)
	require.NotEqual(t, historicalBefore.ID, storedCanonicalMessageID(t, repo, textEvent.StreamID, textEvent.Sequence),
		"the post-upgrade segment must not append to an already projected transcript row")
	var historicalAfter struct {
		ID        string    `db:"id"`
		Content   string    `db:"content"`
		Metadata  string    `db:"metadata"`
		UpdatedAt time.Time `db:"updated_at"`
	}
	require.NoError(t, database.Get(&historicalAfter, database.Rebind("SELECT id, content, metadata, updated_at FROM task_session_messages WHERE id = ?"), legacyMessage.ID))
	require.Equal(t, historicalBefore, historicalAfter, "upgrade projection must preserve existing transcript history")

	repo, err = NewWithDB(database, database, nil)
	require.NoError(t, err, "reopen must not reset a committed boundary")
	requireSegmentBoundary(t, database, textEvent.StreamID, 5)
	textContinuation := canonicalTextSegmentEventFor(t, "segment-upgrade-session", "segment-upgrade-incarnation", textEvent.StreamID, 7, " continued")
	textContinuation.HarnessGeneration = 3
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, textContinuation, textContinuation.Sequence)
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, textContinuation, nil)
	require.NoError(t, err)
	requireSegmentBoundary(t, database, textEvent.StreamID, 5)

	toolEvent := canonicalToolBoundaryEventFor(t, "segment-upgrade-session", "segment-upgrade-incarnation", "segment-upgrade-tool-first", 6)
	toolEvent.HarnessGeneration = 3
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, toolEvent, toolEvent.Sequence)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, toolEvent, nil)
	require.NoError(t, err)
	requireSegmentBoundary(t, database, toolEvent.StreamID, 6)

	repo, err = NewWithDB(database, database, nil)
	require.NoError(t, err)
	requireSegmentBoundary(t, database, toolEvent.StreamID, 6)
	toolContinuation := canonicalTextSegmentEventFor(t, "segment-upgrade-session", "segment-upgrade-incarnation", toolEvent.StreamID, 7, "new segment")
	toolContinuation.HarnessGeneration = 3
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, toolContinuation, toolContinuation.Sequence)
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, toolContinuation, nil)
	require.NoError(t, err)
	requireSegmentBoundary(t, database, toolEvent.StreamID, 6)

	for _, streamID := range []string{textEvent.StreamID, toolEvent.StreamID} {
		cursor, cursorErr := repo.GetAgentDeliveryCursor(ctx, streamID)
		require.NoError(t, cursorErr)
		require.Equal(t, "segment-upgrade-session", cursor.SessionID)
		require.Equal(t, "segment-upgrade-incarnation", cursor.IncarnationID)
		require.EqualValues(t, 3, cursor.HarnessGeneration)
	}
}

func seedLegacySegmentCursor(t *testing.T, database *sqlx.DB, streamID string, now time.Time) {
	t.Helper()
	_, err := database.Exec(database.Rebind("INSERT INTO agent_delivery_cursors (stream_id, session_id, incarnation_id, harness_generation, received_sequence, projected_sequence, remote_high_water, updated_at) VALUES (?, ?, ?, 3, 5, 5, 5, ?)"),
		streamID, "segment-upgrade-session", "segment-upgrade-incarnation", now)
	require.NoError(t, err)
	for sequence := int64(1); sequence <= 5; sequence++ {
		_, err = database.Exec(database.Rebind("INSERT INTO agent_delivery_inbox (stream_id, sequence, session_id, incarnation_id, harness_generation, event_type, payload, terminal, received_at, projected_at, payload_pruned_at) VALUES (?, ?, ?, ?, 3, ?, ?, 0, ?, ?, ?)"),
			streamID, sequence, "segment-upgrade-session", "segment-upgrade-incarnation",
			streams.EventTypeMessageChunk, []byte{}, now, now, now)
		require.NoError(t, err)
	}
}

func assertNullSegmentBoundary(database *sqlx.DB, streamID string) error {
	var boundary sql.NullInt64
	err := database.Get(&boundary, database.Rebind("SELECT message_segment_boundary_sequence FROM agent_delivery_cursors WHERE stream_id = ?"), streamID)
	if err != nil {
		return err
	}
	if boundary.Valid {
		return fmt.Errorf("stream %s boundary initialized to %d before first projection", streamID, boundary.Int64)
	}
	return nil
}

func requireSegmentBoundary(t *testing.T, database *sqlx.DB, streamID string, expected int64) {
	t.Helper()
	var boundary sql.NullInt64
	require.NoError(t, database.Get(&boundary, database.Rebind("SELECT message_segment_boundary_sequence FROM agent_delivery_cursors WHERE stream_id = ?"), streamID))
	require.True(t, boundary.Valid, "boundary for %s must be initialized", streamID)
	require.Equal(t, expected, boundary.Int64)
}

func canonicalTextSegmentEvent(t *testing.T, sequence int64, text string) *models.AgentDeliveryEvent {
	t.Helper()
	return canonicalTextSegmentEventFor(t, "segment-session", "segment-incarnation", "segment-stream", sequence, text)
}

func canonicalTextSegmentEventFor(t *testing.T, sessionID, incarnationID, streamID string, sequence int64, text string) *models.AgentDeliveryEvent {
	t.Helper()
	return canonicalSegmentEventFor(t, sessionID, incarnationID, streamID, sequence, streams.AgentEvent{
		Type: streams.EventTypeMessageChunk, Text: text,
		CanonicalMessageID: "stable-text-base",
	})
}

func canonicalToolBoundaryEvent(t *testing.T, sequence int64, _parentToolCallID, _eventType string) *models.AgentDeliveryEvent {
	t.Helper()
	return canonicalToolBoundaryEventFor(t, "segment-session", "segment-incarnation", "segment-stream", sequence)
}

func canonicalToolBoundaryEventFor(t *testing.T, sessionID, incarnationID, streamID string, sequence int64) *models.AgentDeliveryEvent {
	t.Helper()
	return canonicalSegmentEventFor(t, sessionID, incarnationID, streamID, sequence, streams.AgentEvent{
		Type: streams.EventTypeToolCall, ToolCallID: "tool",
	})
}

func canonicalSegmentEventFor(t *testing.T, sessionID, incarnationID, streamID string, sequence int64, agentEvent streams.AgentEvent) *models.AgentDeliveryEvent {
	t.Helper()
	payload, err := json.Marshal(agentEvent)
	require.NoError(t, err)
	return &models.AgentDeliveryEvent{
		SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 1,
		StreamID: streamID, Sequence: sequence, EventType: agentEvent.Type, Payload: payload,
	}
}

func assertAgentTextSegmentRows(t *testing.T, repo *Repository, sessionID string, expectedContents []string) {
	t.Helper()
	var contents []string
	require.NoError(t, repo.db.Select(&contents, `
		SELECT content FROM task_session_messages
		WHERE task_session_id = ? AND author_type = ? AND type = ? ORDER BY created_at, id`,
		sessionID, string(models.MessageAuthorAgent), string(models.MessageTypeMessage)))
	require.ElementsMatch(t, expectedContents, contents)
	require.Len(t, contents, len(expectedContents))
}

func storedCanonicalMessageID(t *testing.T, repo *Repository, streamID string, sequence int64) string {
	t.Helper()
	var messageID string
	require.NoError(t, repo.db.Get(&messageID, repo.db.Rebind("SELECT canonical_message_id FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?"), streamID, sequence))
	return messageID
}
