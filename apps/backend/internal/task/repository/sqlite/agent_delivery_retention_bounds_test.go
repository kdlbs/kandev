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

func TestDeliveryRetentionBounds(t *testing.T) {
	t.Run("million_event_prefix_frontier", func(t *testing.T) {
		now := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
		ageCutoff := now.Add(-agentDeliveryRetentionCompactMaxAge)
		projectedAt := sql.NullTime{Time: now.Add(-26 * time.Hour), Valid: true}
		firstTerminal := sql.NullInt64{}
		const projectedSequence = int64(1_000_000)
		retired := int64(0)
		for sequence := int64(1); sequence <= projectedSequence; sequence++ {
			if !agentDeliveryRetentionSequenceExpired(
				sequence, retired+1, projectedSequence, firstTerminal, projectedAt, ageCutoff,
				agentDeliveryRetentionCompactEventLimit,
			) {
				t.Fatalf("terminal-free sequence %d was pinned before the million-event frontier", sequence)
			}
			retired = sequence
		}
		if retired != projectedSequence {
			t.Fatalf("terminal-free frontier = %d, want %d", retired, projectedSequence)
		}

		firstTerminal = sql.NullInt64{Int64: 500_000, Valid: true}
		retired = 0
		for sequence := int64(1); sequence <= projectedSequence; sequence++ {
			if !agentDeliveryRetentionSequenceExpired(
				sequence, retired+1, projectedSequence, firstTerminal, projectedAt, ageCutoff,
				agentDeliveryRetentionCompactEventLimit,
			) {
				break
			}
			retired = sequence
		}
		if retired != firstTerminal.Int64-1 {
			t.Fatalf("first-terminal frontier = %d, want %d", retired, firstTerminal.Int64-1)
		}
	})

	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine+"/first_terminal_pins_prefix", func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "terminal", 3, 2)
			result, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{})
			if err != nil {
				t.Fatalf("run terminal retention batch: %v", err)
			}
			if result.Candidates != 3 || result.EventPayloadsPruned != 2 || result.EventRowsRetired != 1 {
				t.Fatalf("terminal retention result = %+v", result)
			}
			cursor, err := repo.GetAgentDeliveryCursor(context.Background(), events[0].StreamID)
			if err != nil {
				t.Fatalf("read terminal cursor: %v", err)
			}
			if cursor.RetiredSequence != 1 {
				t.Fatalf("terminal retired sequence = %d, want 1", cursor.RetiredSequence)
			}
			for _, sequence := range []int64{2, 3} {
				var payload []byte
				var pruned sql.NullTime
				if err := repo.db.QueryRowx(repo.db.Rebind(`
					SELECT payload, payload_pruned_at FROM agent_delivery_inbox
					WHERE stream_id = ? AND sequence = ?`), events[0].StreamID, sequence).Scan(&payload, &pruned); err != nil {
					t.Fatalf("read terminal-pinned event %d: %v", sequence, err)
				}
				if sequence == 2 && (len(payload) == 0 || pruned.Valid) {
					t.Error("unsettled terminal was compacted")
				}
				if sequence == 3 && (len(payload) != 0 || !pruned.Valid) {
					t.Error("independently projected output was not compacted")
				}
			}
		})

		t.Run(engine+"/candidate_and_byte_caps", func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "limits", 5, 0)
			byteLimit := int64(len(events[0].Payload)*2 - 1)
			result, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{
				MaxCandidates: 3, MaxPayloadBytes: byteLimit,
			})
			if err != nil {
				t.Fatalf("run bounded retention batch: %v", err)
			}
			if result.Candidates != 3 {
				t.Fatalf("candidate count = %d, want 3", result.Candidates)
			}
			if result.PayloadBytesRead > byteLimit || result.PayloadBytesPruned > byteLimit {
				t.Fatalf("retention exceeded byte budget %d: %+v", byteLimit, result)
			}
			if result.PayloadBytesRead == 0 || result.EventPayloadsPruned != 1 {
				t.Fatalf("bounded retention did not use one payload slot: %+v", result)
			}
			for _, sequence := range []int64{4, 5} {
				var pruned sql.NullTime
				if err := repo.db.QueryRowx(repo.db.Rebind(`
					SELECT payload_pruned_at FROM agent_delivery_inbox
					WHERE stream_id = ? AND sequence = ?`), events[0].StreamID, sequence).Scan(&pruned); err != nil {
					t.Fatalf("read event %d after bounded scan: %v", sequence, err)
				}
				if pruned.Valid {
					t.Errorf("event %d was processed beyond candidate cap", sequence)
				}
			}
		})
	}
}

func seedRetentionBoundsStream(
	t *testing.T,
	engine, suffix string,
	count int,
	terminalSequence int,
) (*Repository, []*models.AgentDeliveryEvent) {
	t.Helper()
	repo := newRetentionTestRepo(t, engine)
	ctx := context.Background()
	taskID := "retention-bounds-task-" + suffix
	sessionID := "retention-bounds-session-" + suffix
	streamID := "retention-bounds-stream-" + suffix
	baseTime := time.Date(2025, 4, 5, 6, 7, 8, 0, time.UTC)
	currentTime := baseTime
	repo.clockNow = func() time.Time { return currentTime }
	workspaceID := retentionTestWorkspaceID(t, repo)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Retention bounds"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, QueueIncarnationID: "incarnation-" + sessionID,
		State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
	events := make([]*models.AgentDeliveryEvent, 0, count)
	for index := 0; index < count; index++ {
		sequence := index + 1
		eventType := streams.EventTypeMessageChunk
		terminal := sequence == terminalSequence
		if terminal {
			eventType = streams.EventTypeComplete
		}
		payload, err := json.Marshal(streams.AgentEvent{Type: eventType, Text: "x"})
		if err != nil {
			t.Fatalf("marshal event %d: %v", sequence, err)
		}
		event := &models.AgentDeliveryEvent{
			SessionID: sessionID, IncarnationID: "incarnation-" + sessionID,
			HarnessGeneration: 1, StreamID: streamID, Sequence: int64(sequence),
			EventType: eventType, Payload: payload, Terminal: terminal, ReceivedAt: baseTime,
		}
		if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, int64(sequence)); err != nil || !inserted {
			t.Fatalf("receive event %d = %v, err %v", sequence, inserted, err)
		}
		events = append(events, event)
	}
	effects := make([]*models.AgentDeliveryEffect, 0, len(events))
	for _, event := range events {
		effects = append(effects, completedRetentionEventEffect(event))
	}
	if _, err := repo.ProjectCanonicalAgentDeliveryEvents(ctx, events, effects); err != nil {
		t.Fatalf("project events: %v", err)
	}
	currentTime = baseTime.Add(26 * time.Hour)
	return repo, events
}
