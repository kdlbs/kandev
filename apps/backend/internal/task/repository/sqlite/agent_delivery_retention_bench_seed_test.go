package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

const retentionBenchmarkBacklog = 100_000

func seedRetentionBenchmarkBacklog(b *testing.B, repo *Repository) *models.AgentDeliveryEvent {
	b.Helper()
	ctx := context.Background()
	old := time.Now().UTC().Add(-48 * time.Hour)
	payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "benchmark old output"})
	if err != nil {
		b.Fatal(err)
	}
	event := &models.AgentDeliveryEvent{SessionID: "retention-benchmark-session", IncarnationID: "retention-benchmark-incarnation", HarnessGeneration: 1, StreamID: "old-retention-benchmark-stream", Sequence: 1, EventType: streams.EventTypeMessageChunk, Payload: payload, ReceivedAt: old}
	if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 1); err != nil || !inserted {
		b.Fatalf("seed old event: %v %v", inserted, err)
	}
	effect := &models.AgentDeliveryEffect{EffectKey: fmt.Sprintf("agent_delivery.event:%s:1", event.StreamID), StreamID: event.StreamID, Sequence: 1, EffectType: "agent_delivery.event", State: models.DeliveryEffectCompleted, CreatedAt: old, CompletedAt: &old}
	if _, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, effect); err != nil {
		b.Fatal(err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_inbox SET projected_at = ? WHERE stream_id = ?`), old, event.StreamID); err != nil {
		b.Fatal(err)
	}
	seedRetentionBenchmarkRows(b, repo, event)
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_cursors SET received_sequence = ?, projected_sequence = ?, remote_high_water = ? WHERE stream_id = ?`), retentionBenchmarkBacklog, retentionBenchmarkBacklog, retentionBenchmarkBacklog, event.StreamID); err != nil {
		b.Fatal(err)
	}
	if count := retentionBenchmarkEligibleBacklog(b, repo, event.StreamID); count != retentionBenchmarkBacklog {
		b.Fatalf("seeded eligible backlog=%d want=%d", count, retentionBenchmarkBacklog)
	}
	return event
}

func seedRetentionBenchmarkRows(b *testing.B, repo *Repository, event *models.AgentDeliveryEvent) {
	b.Helper()
	payload := []byte(`{"type":"message_chunk","text":"` + strings.Repeat("x", 990) + `"}`)
	if len(payload) != 1024 {
		b.Fatalf("benchmark payload bytes=%d want=1024", len(payload))
	}
	for first := 2; first <= retentionBenchmarkBacklog; first += 10000 {
		last := first + 9999
		if last > retentionBenchmarkBacklog {
			last = retentionBenchmarkBacklog
		}
		seedRetentionBenchmarkRange(b, repo, event, first, last, payload)
	}
}

func seedRetentionBenchmarkRange(b *testing.B, repo *Repository, event *models.AgentDeliveryEvent, first, last int, payload []byte) {
	b.Helper()
	tx, err := repo.db.Beginx()
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(repo.db.Rebind(`WITH RECURSIVE sequences(n) AS (
	 SELECT CAST(? AS BIGINT) UNION ALL SELECT n+1 FROM sequences WHERE n < ?
	) INSERT INTO agent_delivery_inbox
	 (stream_id, sequence, session_id, incarnation_id, harness_generation, submission_id, event_type, payload, terminal, received_at, projected_at)
	 SELECT ?, n, ?, ?, 1, ?, 'message_chunk', ?, 0, ?, ? FROM sequences`), first, last, event.StreamID, event.SessionID, event.IncarnationID, event.SubmissionID, payload, event.ReceivedAt, event.ReceivedAt)
	if err == nil {
		_, err = tx.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_effects
		 (effect_key, stream_id, sequence, effect_type, state, created_at, completed_at)
		 SELECT 'agent_delivery.event:' || stream_id || ':' || CAST(sequence AS TEXT), stream_id, sequence,
		 'agent_delivery.event', 'completed', received_at, projected_at
		 FROM agent_delivery_inbox WHERE stream_id = ? AND sequence BETWEEN ? AND ?`), event.StreamID, first, last)
	}
	if err != nil {
		b.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
}

func retentionBenchmarkEligibleBacklog(b *testing.B, repo *Repository, streamID string) int64 {
	b.Helper()
	var count int64
	if err := repo.db.QueryRow(repo.db.Rebind(`SELECT COUNT(*) FROM agent_delivery_inbox WHERE stream_id = ? AND LENGTH(payload) > 0 AND projected_at <= ?`), streamID, time.Now().UTC().Add(-time.Hour)).Scan(&count); err != nil {
		b.Fatal(err)
	}
	return count
}
