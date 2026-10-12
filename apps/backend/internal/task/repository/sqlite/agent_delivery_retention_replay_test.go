package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionHash(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			seedAgentDeliveryTestSession(t, repo, "retention-hash-session", "retention-hash-incarnation")
			ctx := context.Background()
			original := retentionHashEvent(t, 1, streams.AgentEvent{
				Type:               streams.EventTypeMessageChunk,
				Text:               "retained text",
				ControlTurnID:      7,
				PromptGeneration:   1,
				TurnID:             "old-turn",
				CanonicalMessageID: "old-message",
			})
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, original, 1); err != nil || !inserted {
				t.Fatalf("receive original event = %v, err %v", inserted, err)
			}
			if original.HashVersion != 1 || original.ContentHash == "" {
				t.Fatalf("stored receipt metadata = version %d, hash %q", original.HashVersion, original.ContentHash)
			}

			reconstructed := retentionHashEvent(t, 1, streams.AgentEvent{
				Type:                   streams.EventTypeMessageChunk,
				Text:                   "retained text",
				PromptGeneration:       1,
				TurnID:                 "new-turn",
				CanonicalMessageID:     "new-message",
				CanonicalProjection:    true,
				CanonicalMessageAppend: true,
			})
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, reconstructed, 1); err != nil || inserted {
				t.Fatalf("receive reconstructed duplicate = %v, err %v", inserted, err)
			}
			if reconstructed.ContentHash != original.ContentHash {
				t.Fatalf("normalized replay hashes differ: %q != %q", reconstructed.ContentHash, original.ContentHash)
			}

			conflicting := retentionHashEvent(t, 1, streams.AgentEvent{
				Type: streams.EventTypeMessageChunk,
				Text: "changed adapter content",
			})
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, conflicting, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryEventConflict) || inserted {
				t.Fatalf("receive changed content = %v, err %v", inserted, err)
			}

			malformed := &models.AgentDeliveryEvent{
				SessionID: "retention-hash-session", IncarnationID: "retention-hash-incarnation",
				HarnessGeneration: 1, StreamID: "retention-raw-stream", Sequence: 1,
				EventType: "adapter_event", Payload: []byte(`{"broken":`),
			}
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, malformed, 1); err != nil || !inserted {
				t.Fatalf("receive malformed payload = %v, err %v", inserted, err)
			}
			if malformed.HashVersion != 1 || malformed.ContentHash == "" {
				t.Fatalf("malformed receipt metadata = version %d, hash %q", malformed.HashVersion, malformed.ContentHash)
			}
			malformedReplay := *malformed
			malformedReplay.ContentHash = ""
			malformedReplay.HashVersion = 0
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &malformedReplay, 1); err != nil || inserted {
				t.Fatalf("receive identical malformed replay = %v, err %v", inserted, err)
			}
			malformedReplay.Payload = []byte(`{ "broken":`)
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &malformedReplay, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryEventConflict) || inserted {
				t.Fatalf("receive altered malformed replay = %v, err %v", inserted, err)
			}

			unsupported := retentionHashEvent(t, 1, streams.AgentEvent{
				Type: streams.EventTypeMessageChunk, Text: "unknown hash version",
			})
			unsupported.StreamID = "retention-unknown-hash-stream"
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, unsupported, 1); err != nil || !inserted {
				t.Fatalf("receive supported event before version mutation = %v, err %v", inserted, err)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`
				UPDATE agent_delivery_inbox SET hash_version = 99
				WHERE stream_id = ? AND sequence = 1`), unsupported.StreamID); err != nil {
				t.Fatalf("set unsupported stored hash version: %v", err)
			}
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, unsupported, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryHashVersion) || inserted {
				t.Fatalf("replay unknown hash version = %v, err %v", inserted, err)
			}
		})
	}
}

func TestDeliveryRetentionHashEnvelope(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			seedAgentDeliveryTestSession(t, repo, "retention-hash-session", "retention-hash-incarnation")
			baseline := retentionHashEvent(t, 1, streams.AgentEvent{
				Type: streams.EventTypeMessageChunk, Text: "same transport content",
				Data: map[string]any{"provider_diagnostic": "source-only diagnostic"},
			})
			if inserted, err := repo.ReceiveAgentDeliveryEvent(context.Background(), baseline, 1); err != nil || !inserted {
				t.Fatalf("receive baseline event = %v, err %v", inserted, err)
			}
			mutations := []struct {
				name   string
				change func(*models.AgentDeliveryEvent)
			}{
				{name: "session", change: func(event *models.AgentDeliveryEvent) { event.SessionID = "another-session" }},
				{name: "incarnation", change: func(event *models.AgentDeliveryEvent) { event.IncarnationID = "another-incarnation" }},
				{name: "harness-generation", change: func(event *models.AgentDeliveryEvent) { event.HarnessGeneration++ }},
				{name: "submission", change: func(event *models.AgentDeliveryEvent) { event.SubmissionID = "prompt:other" }},
				{name: "event-type", change: func(event *models.AgentDeliveryEvent) { event.EventType = "tool_update" }},
				{name: "terminal", change: func(event *models.AgentDeliveryEvent) { event.Terminal = true }},
			}
			for _, mutation := range mutations {
				t.Run(mutation.name, func(t *testing.T) {
					candidate := *baseline
					mutation.change(&candidate)
					_, receiveErr := repo.ReceiveAgentDeliveryEvent(context.Background(), &candidate, 1)
					if receiveErr == nil {
						t.Fatal("changed identity envelope was accepted")
					}
					if candidate.ContentHash == baseline.ContentHash {
						t.Fatalf("%s envelope mutation did not change receipt hash %q", mutation.name, candidate.ContentHash)
					}
				})
			}
			for _, mutation := range []struct {
				name   string
				change func(*models.AgentDeliveryEvent)
			}{
				{name: "stream", change: func(event *models.AgentDeliveryEvent) { event.StreamID = "another-stream" }},
				{name: "sequence", change: func(event *models.AgentDeliveryEvent) { event.Sequence++ }},
			} {
				candidate := *baseline
				mutation.change(&candidate)
				if got := agentDeliveryEventHash(&candidate); got == baseline.ContentHash {
					t.Errorf("%s envelope mutation did not change receipt hash %q", mutation.name, got)
				}
			}

			changedDiagnostic := retentionHashEvent(t, 1, streams.AgentEvent{
				Type: streams.EventTypeMessageChunk, Text: "same transport content",
				Data: map[string]any{"provider_diagnostic": "changed source diagnostic"},
			})
			if inserted, err := repo.ReceiveAgentDeliveryEvent(context.Background(), changedDiagnostic, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryEventConflict) || inserted {
				t.Fatalf("changed source diagnostic replay = %v, err %v", inserted, err)
			}
		})
	}
}

func TestDeliveryRetentionReplay(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			seedAgentDeliveryTestSession(t, repo, "retention-replay-session", "retention-replay-incarnation")
			ctx := context.Background()
			event := &models.AgentDeliveryEvent{
				SessionID: "retention-replay-session", IncarnationID: "retention-replay-incarnation",
				HarnessGeneration: 2, StreamID: "retention-replay-stream", Sequence: 1,
				SubmissionID: "prompt:retention-replay", EventType: "assistant_message",
				Payload: []byte("retained payload"),
			}
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 1); err != nil || !inserted {
				t.Fatalf("receive event = %v, err %v", inserted, err)
			}
			effect := &models.AgentDeliveryEffect{
				EffectKey: "retention-replay-effect", StreamID: event.StreamID, Sequence: event.Sequence,
				EffectType: "agent_delivery.event", State: models.DeliveryEffectCompleted,
				SessionID: event.SessionID, IncarnationID: event.IncarnationID,
				HarnessGeneration: event.HarnessGeneration, SubmissionID: event.SubmissionID,
				CreatedAt: time.Now().UTC(),
			}
			if _, err := repo.ProjectAgentDeliveryEvent(ctx, event, effect); err != nil {
				t.Fatalf("project event: %v", err)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`
				UPDATE agent_delivery_inbox
				SET payload = ?, payload_pruned_at = ?, payload_bytes = ?
				WHERE stream_id = ? AND sequence = ?`),
				[]byte{}, time.Now().UTC(), len(event.Payload), event.StreamID, event.Sequence); err != nil {
				t.Fatalf("compact inbox payload: %v", err)
			}

			replay := *event
			replay.Payload = append([]byte(nil), event.Payload...)
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &replay, 1); err != nil || inserted {
				t.Fatalf("receive compact duplicate = %v, err %v", inserted, err)
			}
			if _, err := repo.ProjectAgentDeliveryEvent(ctx, &replay, effect); err != nil {
				t.Fatalf("project compact duplicate: %v", err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ?", event.StreamID, 1)
			assertRetentionRowCount(t, repo, "agent_delivery_effects", "effect_key = ?", effect.EffectKey, 1)

			conflict := replay
			conflict.Payload = []byte("different payload")
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &conflict, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryEventConflict) || inserted {
				t.Fatalf("receive conflicting compact duplicate = %v, err %v", inserted, err)
			}
			conflict.IncarnationID = "foreign-incarnation"
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &conflict, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) || inserted {
				t.Fatalf("receive compact duplicate with foreign identity = %v, err %v", inserted, err)
			}
		})
	}
}

func TestDeliveryRetentionExpiredReplay(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			seedAgentDeliveryTestSession(t, repo, "retention-expired-session", "retention-expired-incarnation")
			event := &models.AgentDeliveryEvent{
				SessionID: "retention-expired-session", IncarnationID: "retention-expired-incarnation",
				HarnessGeneration: 1, StreamID: "retention-expired-stream", Sequence: 1,
				EventType: "assistant_message", Payload: []byte("expired payload"),
			}
			if inserted, err := repo.ReceiveAgentDeliveryEvent(context.Background(), event, 1); err != nil || !inserted {
				t.Fatalf("receive event = %v, err %v", inserted, err)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`
				UPDATE agent_delivery_cursors SET retired_sequence = 1 WHERE stream_id = ?`), event.StreamID); err != nil {
				t.Fatalf("advance retirement checkpoint: %v", err)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`
				DELETE FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?`), event.StreamID, event.Sequence); err != nil {
				t.Fatalf("delete retired receipt: %v", err)
			}
			if inserted, err := repo.ReceiveAgentDeliveryEvent(context.Background(), event, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryReplayExpired) || inserted {
				t.Fatalf("expired replay = %v, err %v; want a typed rejection before insertion", inserted, err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ?", event.StreamID, 0)
		})
	}
}

func TestDeliveryRetentionCheckpointMissing(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			seedAgentDeliveryTestSession(t, repo, "retention-missing-session", "retention-missing-incarnation")
			event := &models.AgentDeliveryEvent{
				SessionID: "retention-missing-session", IncarnationID: "retention-missing-incarnation",
				HarnessGeneration: 1, StreamID: "retention-missing-stream", Sequence: 1,
				EventType: "assistant_message", Payload: []byte("retained payload"),
			}
			if inserted, err := repo.ReceiveAgentDeliveryEvent(context.Background(), event, 1); err != nil || !inserted {
				t.Fatalf("receive event = %v, err %v", inserted, err)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`DELETE FROM agent_delivery_cursors WHERE stream_id = ?`), event.StreamID); err != nil {
				t.Fatalf("delete checkpoint: %v", err)
			}
			if inserted, err := repo.ReceiveAgentDeliveryEvent(context.Background(), event, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryCheckpointMissing) || inserted {
				t.Fatalf("replay without cursor = %v, err %v; want fail-closed checkpoint error", inserted, err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_cursors", "stream_id = ?", event.StreamID, 0)

			missingRow := *event
			missingRow.StreamID = "retention-missing-row-stream"
			if inserted, err := repo.ReceiveAgentDeliveryEvent(context.Background(), &missingRow, 1); err != nil || !inserted {
				t.Fatalf("receive event before row loss = %v, err %v", inserted, err)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`
				DELETE FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?`), missingRow.StreamID, missingRow.Sequence); err != nil {
				t.Fatalf("delete receipt above retirement checkpoint: %v", err)
			}
			if inserted, err := repo.ReceiveAgentDeliveryEvent(context.Background(), &missingRow, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryCheckpointMissing) || inserted {
				t.Fatalf("replay with missing received row = %v, err %v; want fail-closed checkpoint error", inserted, err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ?", missingRow.StreamID, 0)
		})
	}
}

func retentionHashEvent(t *testing.T, sequence int64, wire streams.AgentEvent) *models.AgentDeliveryEvent {
	t.Helper()
	payload, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	return &models.AgentDeliveryEvent{
		SessionID: "retention-hash-session", IncarnationID: "retention-hash-incarnation",
		HarnessGeneration: 1, StreamID: "retention-hash-stream", Sequence: sequence,
		EventType: wire.Type, Payload: payload,
	}
}

func assertRetentionRowCount(t *testing.T, repo *Repository, table, predicate string, value any, want int) {
	t.Helper()
	query := "SELECT COUNT(*) FROM " + table + " WHERE " + predicate
	var got int
	if err := repo.db.QueryRowx(repo.db.Rebind(query), value).Scan(&got); err != nil && err != sql.ErrNoRows {
		t.Fatalf("count %s rows: %v", table, err)
	}
	if got != want {
		t.Errorf("%s row count = %d, want %d", table, got, want)
	}
}
