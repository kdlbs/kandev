package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionEvents(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			ctx := context.Background()
			const (
				taskID    = "retention-events-task"
				sessionID = "retention-events-session"
				streamID  = "retention-events-stream"
			)
			baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
			currentTime := baseTime
			repo.clockNow = func() time.Time { return currentTime }
			workspaceID := retentionTestWorkspaceID(t, repo)
			if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Retention events"}); err != nil {
				t.Fatalf("create task: %v", err)
			}
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: sessionID, TaskID: taskID, QueueIncarnationID: "incarnation-" + sessionID,
				State: models.TaskSessionStateCreated,
			}); err != nil {
				t.Fatalf("create task session: %v", err)
			}

			types := []string{
				streams.EventTypeMessageChunk,
				streams.EventTypePermissionRequest,
				streams.EventTypeMessageChunk,
				streams.EventTypeMessageChunk,
				streams.EventTypeMessageChunk,
				streams.EventTypeMessageChunk,
			}
			events := make([]*models.AgentDeliveryEvent, 0, len(types))
			for index, eventType := range types {
				wire := streams.AgentEvent{Type: eventType, Text: "retained output " + string(rune('a'+index))}
				payload, err := json.Marshal(wire)
				if err != nil {
					t.Fatalf("marshal event %d: %v", index+1, err)
				}
				event := &models.AgentDeliveryEvent{
					SessionID: sessionID, IncarnationID: "incarnation-" + sessionID,
					HarnessGeneration: 1, StreamID: streamID, Sequence: int64(index + 1),
					EventType: eventType, Payload: payload, ReceivedAt: baseTime,
				}
				if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, int64(index+1)); err != nil || !inserted {
					t.Fatalf("receive event %d = %v, err %v", index+1, inserted, err)
				}
				events = append(events, event)
			}

			effects := make([]*models.AgentDeliveryEffect, 0, 5)
			for _, event := range events[:5] {
				effect := completedRetentionEventEffect(event)
				if event.Sequence == 4 {
					effect.State = models.DeliveryEffectPending
					effect.CompletedAt = nil
				}
				effects = append(effects, effect)
			}
			if _, err := repo.ProjectCanonicalAgentDeliveryEvents(ctx, events[:5], effects); err != nil {
				t.Fatalf("project first five events: %v", err)
			}
			oldProjection := baseTime.Add(-25 * time.Hour)
			if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
				UPDATE agent_delivery_inbox SET projected_at = ?
				WHERE stream_id = ? AND sequence BETWEEN 1 AND 5`), oldProjection, streamID); err != nil {
				t.Fatalf("age projected events: %v", err)
			}
			unknownEffect := &models.AgentDeliveryEffect{
				EffectKey: "future.retention-effect:" + streamID + ":5", StreamID: streamID,
				Sequence: 5, EffectType: "future.retention-effect", State: models.DeliveryEffectCompleted,
				CreatedAt: baseTime, CompletedAt: &baseTime,
			}
			if _, err := repo.PutAgentDeliveryEffect(ctx, unknownEffect); err != nil {
				t.Fatalf("store unknown effect: %v", err)
			}
			workflowReceipt := &models.AgentDeliveryEffect{
				EffectKey: "workflow.on_turn_complete:retention-events-turn", StreamID: streamID,
				Sequence: 1, EffectType: "workflow.on_turn_complete", State: models.DeliveryEffectCompleted,
				SessionID: sessionID, IncarnationID: "incarnation-" + sessionID,
				HarnessGeneration: 1, TurnID: "retention-events-turn",
				CreatedAt: baseTime, CompletedAt: &baseTime,
			}
			if _, err := repo.PutAgentDeliveryEffect(ctx, workflowReceipt); err != nil {
				t.Fatalf("store workflow receipt: %v", err)
			}
			currentTime = baseTime.Add(26 * time.Hour)

			result, err := repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{})
			if err != nil {
				t.Fatalf("run retention batch: %v", err)
			}
			if result.Candidates != 5 || result.EventPayloadsPruned != 2 || result.EventRowsRetired != 1 {
				t.Fatalf("retention result = %+v, want five projected candidates, two payloads pruned, one event retired", result)
			}

			var payload []byte
			var hashVersion int
			var contentHash string
			var payloadPruned sql.NullTime
			var payloadBytes int64
			query := repo.db.Rebind(`
				SELECT payload, content_hash, hash_version, payload_pruned_at, payload_bytes
				FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?`)
			if err := repo.db.QueryRowxContext(ctx, query, streamID, 3).Scan(
				&payload, &contentHash, &hashVersion, &payloadPruned, &payloadBytes,
			); err != nil {
				t.Fatalf("read compacted event: %v", err)
			}
			if len(payload) != 0 || !payloadPruned.Valid || hashVersion != agentDeliveryPayloadHashVersion || contentHash == "" || payloadBytes != int64(len(events[2].Payload)) {
				t.Fatalf("compacted event metadata = payload %d bytes, hash %q v%d, pruned %v, original bytes %d", len(payload), contentHash, hashVersion, payloadPruned, payloadBytes)
			}

			for _, sequence := range []int64{2, 4, 5, 6} {
				var gotPayload []byte
				var prunedAt sql.NullTime
				if err := repo.db.QueryRowxContext(ctx, repo.db.Rebind(`
					SELECT payload, payload_pruned_at FROM agent_delivery_inbox
					WHERE stream_id = ? AND sequence = ?`), streamID, sequence).Scan(&gotPayload, &prunedAt); err != nil {
					t.Fatalf("read protected event %d: %v", sequence, err)
				}
				if len(gotPayload) == 0 || prunedAt.Valid {
					t.Errorf("protected event %d was pruned", sequence)
				}
			}

			cursor, err := repo.GetAgentDeliveryCursor(ctx, streamID)
			if err != nil {
				t.Fatalf("read cursor: %v", err)
			}
			if cursor.ReceivedSequence != 6 || cursor.ProjectedSequence != 5 || cursor.RetiredSequence != 1 {
				t.Errorf("delivery cursor after retention = %+v", cursor)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_effects", "effect_key = ?", "agent_delivery.event:"+streamID+":1", 0)
			assertRetentionRowCount(t, repo, "agent_delivery_effects", "effect_key = ?", workflowReceipt.EffectKey, 1)
			assertRetentionRowCount(t, repo, "agent_delivery_effects", "effect_key = ?", unknownEffect.EffectKey, 1)

			originalReplay := *events[2]
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &originalReplay, 5); err != nil || inserted {
				t.Errorf("replay compacted-but-retained event = %v, err %v", inserted, err)
			}
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, events[0], 5); err == nil || inserted {
				t.Errorf("replay retired event = %v, err %v, want expired replay", inserted, err)
			}

			for _, sequence := range []int64{1, 3} {
				var content string
				messageID := "agent-delivery-" + streamID + "-" + string(rune('0'+sequence))
				if err := repo.db.QueryRowxContext(ctx, repo.db.Rebind(`
					SELECT content FROM task_session_messages WHERE id = ?`), messageID).Scan(&content); err != nil {
					t.Fatalf("read canonical message %d: %v", sequence, err)
				}
				if content == "" {
					t.Errorf("canonical message %d was removed with transport payload", sequence)
				}
			}
		})
	}
}

func completedRetentionEventEffect(event *models.AgentDeliveryEvent) *models.AgentDeliveryEffect {
	completedAt := event.ReceivedAt
	return &models.AgentDeliveryEffect{
		EffectKey:   "agent_delivery.event:" + event.StreamID + ":" + string(rune('0'+event.Sequence)),
		StreamID:    event.StreamID,
		Sequence:    event.Sequence,
		EffectType:  "agent_delivery.event",
		State:       models.DeliveryEffectCompleted,
		CreatedAt:   event.ReceivedAt,
		CompletedAt: &completedAt,
	}
}

func retentionTestWorkspaceID(t *testing.T, repo *Repository) string {
	t.Helper()
	var workspaceID string
	if err := repo.db.QueryRowx(`SELECT id FROM workspaces ORDER BY created_at LIMIT 1`).Scan(&workspaceID); err != nil {
		t.Fatalf("read default retention test workspace: %v", err)
	}
	return workspaceID
}
