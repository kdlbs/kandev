package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DELIVERY-RETENTION-005.7
func TestDeliveryRetentionControlPrefix(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionControls(t, engine)
			require.NoError(t, repo.SetSessionMetadataKey(t.Context(), events[0].SessionID, "acp", map[string]any{"session_id": "native-session", "title": "current title"}))
			sessionBefore, err := repo.GetTaskSession(t.Context(), events[0].SessionID)
			require.NoError(t, err)
			before := retentionCanonicalContent(t, repo, events[0].SessionID)
			preview, err := repo.AnalyzeAgentDeliveryRetention(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
			require.NoError(t, err)
			require.Equal(t, len(events), preview.EventReceiptCandidates)
			result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
			require.NoError(t, err)
			require.Equal(t, len(events), result.EventRowsRetired, "bootstrap control must not pin expired output")
			cursor, err := repo.GetAgentDeliveryCursor(t.Context(), events[0].StreamID)
			require.NoError(t, err)
			require.Equal(t, int64(len(events)), cursor.RetiredSequence)
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=?", events[0].StreamID, 0)
			assertRetentionRowCount(t, repo, "agent_delivery_effects", "stream_id=?", events[0].StreamID, 0)
			require.Equal(t, before, retentionCanonicalContent(t, repo, events[0].SessionID))
			sessionAfter, err := repo.GetTaskSession(t.Context(), events[0].SessionID)
			require.NoError(t, err)
			require.Equal(t, sessionBefore.Metadata, sessionAfter.Metadata)
		})
	}
}

func TestDeliveryRetentionControlReplay(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionControls(t, engine)
			repo.clockNow = func() time.Time { return events[0].ReceivedAt.Add(2 * time.Hour) }
			result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
			require.NoError(t, err)
			require.Equal(t, len(events), result.EventPayloadsPruned)
			require.Zero(t, result.EventRowsRetired)
			for _, event := range events {
				inserted, replayErr := repo.ReceiveAgentDeliveryEvent(t.Context(), event, int64(len(events)))
				require.NoError(t, replayErr)
				require.False(t, inserted)
			}
			changed := *events[0]
			changed.Payload = []byte(`{"type":"agent_capabilities","supports_image":false}`)
			_, err = repo.ReceiveAgentDeliveryEvent(t.Context(), &changed, int64(len(events)))
			require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryEventConflict)
			repo.clockNow = func() time.Time { return events[0].ReceivedAt.Add(26 * time.Hour) }
			result, err = repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
			require.NoError(t, err)
			require.Equal(t, len(events), result.EventRowsRetired)
			_, err = repo.ReceiveAgentDeliveryEvent(t.Context(), events[0], int64(len(events)))
			require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryReplayExpired)
		})
	}
}

func TestDeliveryRetentionControlProtectedBoundary(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, guard := range []string{"unknown", "pending", "missing_effect", "unprojected", "user_role", "missing_submission", "unknown_effect", "terminal"} {
			t.Run(engine+"/"+guard, func(t *testing.T) {
				repo, events := seedRetentionControls(t, engine)
				protectRetentionControl(t, repo, events[4], guard)
				result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
				require.NoError(t, err)
				require.Equal(t, 4, result.EventRowsRetired)
				cursor, err := repo.GetAgentDeliveryCursor(t.Context(), events[0].StreamID)
				require.NoError(t, err)
				require.Equal(t, int64(4), cursor.RetiredSequence)
				assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=? AND sequence>=5", events[0].StreamID, len(events)-4)
			})
		}
	}
}

func TestDeliveryRetentionControlRestart(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionControls(t, engine)
			injected := errors.New("control retirement interrupted")
			repo.agentDeliveryRetentionFailpoint = func(point string) error {
				if point == "after_retired_cursor" {
					return injected
				}
				return nil
			}
			_, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
			require.ErrorIs(t, err, injected)
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=? AND payload_pruned_at IS NULL", events[0].StreamID, len(events))
			restarted, err := NewWithDB(repo.db, repo.ro, nil)
			require.NoError(t, err)
			restarted.clockNow = repo.clockNow
			t.Cleanup(func() { _ = restarted.Close() })
			result, err := restarted.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
			require.NoError(t, err)
			require.Equal(t, len(events), result.EventRowsRetired)
		})
	}
}

func seedRetentionControls(t *testing.T, engine string) (*Repository, []*models.AgentDeliveryEvent) {
	t.Helper()
	repo := newRetentionTestRepo(t, engine)
	seedAgentDeliveryTestSession(t, repo, "controls-session", "controls-incarnation")
	base := time.Date(2025, 4, 5, 6, 7, 8, 0, time.UTC)
	repo.clockNow = func() time.Time { return base }
	types := []string{"agent_capabilities", "session_status", "available_commands", "session_mode",
		"session_models", "session_info", "context_window", "mcp_attachment", "turn_started", "message_chunk"}
	events := make([]*models.AgentDeliveryEvent, 0, len(types))
	for index, kind := range types {
		payload, err := json.Marshal(streams.AgentEvent{Type: kind, SessionID: "controls-session", Text: "preserved output", SupportsImage: true})
		require.NoError(t, err)
		event := &models.AgentDeliveryEvent{SessionID: "controls-session", IncarnationID: "controls-incarnation",
			HarnessGeneration: 1, StreamID: "controls-stream", Sequence: int64(index + 1), EventType: kind,
			Payload: payload, ReceivedAt: base}
		inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), event, event.Sequence)
		require.NoError(t, err)
		require.True(t, inserted)
		effect := completedRetentionEventEffect(event)
		effect.EffectKey = "agent_delivery.event:" + event.StreamID + ":" + strconv.FormatInt(event.Sequence, 10)
		if kind == "message_chunk" {
			_, err = repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), event, effect)
		} else {
			_, err = repo.ProjectAgentDeliveryEvent(t.Context(), event, effect)
		}
		require.NoError(t, err)
		events = append(events, event)
	}
	repo.clockNow = func() time.Time { return base.Add(26 * time.Hour) }
	return repo, events
}

func protectRetentionControl(t *testing.T, repo *Repository, event *models.AgentDeliveryEvent, guard string) {
	t.Helper()
	var query string
	switch guard {
	case "pending":
		query = `UPDATE agent_delivery_effects SET state='pending',completed_at=NULL WHERE stream_id=? AND sequence=?`
	case "missing_effect":
		query = `DELETE FROM agent_delivery_effects WHERE stream_id=? AND sequence=?`
	case "unprojected":
		query = `UPDATE agent_delivery_inbox SET projected_at=NULL WHERE stream_id=? AND sequence=?`
	case "unknown_effect":
		effect := completedRetentionEventEffect(event)
		effect.EffectKey, effect.EffectType = "control.unknown_effect", "unknown.control"
		_, err := repo.PutAgentDeliveryEffect(t.Context(), effect)
		require.NoError(t, err)
		return
	case "terminal":
		changed := *event
		changed.EventType, changed.Terminal = streams.EventTypeComplete, true
		changed.Payload = []byte(`{"type":"complete"}`)
		_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`UPDATE agent_delivery_inbox
		 SET event_type=?,terminal=1,payload=?,content_hash=?,payload_bytes=? WHERE stream_id=? AND sequence=?`),
			changed.EventType, changed.Payload, agentDeliveryEventHash(&changed), len(changed.Payload), event.StreamID, event.Sequence)
		require.NoError(t, err)
		return
	case "missing_submission":
		changed := *event
		changed.SubmissionID = "absent"
		_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`UPDATE agent_delivery_inbox
		 SET submission_id=?,content_hash=? WHERE stream_id=? AND sequence=?`),
			changed.SubmissionID, agentDeliveryEventHash(&changed), event.StreamID, event.Sequence)
		require.NoError(t, err)
		return
	default:
		wire := streams.AgentEvent{Type: event.EventType, Role: "user"}
		if guard == "unknown" {
			wire.Type, wire.Role = "provider_extension", ""
		}
		changed := *event
		changed.EventType = wire.Type
		var err error
		changed.Payload, err = json.Marshal(wire)
		require.NoError(t, err)
		_, err = repo.db.ExecContext(context.Background(), repo.db.Rebind(`UPDATE agent_delivery_inbox
		 SET event_type=?,payload=?,content_hash=?,payload_bytes=? WHERE stream_id=? AND sequence=?`),
			wire.Type, changed.Payload, agentDeliveryEventHash(&changed), len(changed.Payload), event.StreamID, event.Sequence)
		require.NoError(t, err)
		return
	}
	_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(query), event.StreamID, event.Sequence)
	require.NoError(t, err)
}
