package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-EXECUTORS-IDLE-PARKING-001.11
func TestIdleActivityRetainedCompletionPreservesObservationAndDedup(t *testing.T) {
	old := time.Now().UTC().Add(-time.Hour)
	for _, known := range []bool{true, false} {
		t.Run(strconv.FormatBool(known), func(t *testing.T) {
			outcome := &agentctl.TurnOutcome{TurnID: 9, Event: agentctl.AgentEvent{Type: "complete", PromptGeneration: 5}}
			if known {
				outcome.Event.ActivityAt = &old
			}
			backend := &fakeTurnOutcomeBackend{MockExecutor: &MockExecutor{name: executor.NameStandalone}}
			mgr, eventBus := newTurnOutcomeTestManager(t, backend)
			backend.eventBus = eventBus
			e := createTestExecution("execution", "task", "session")
			require.NoError(t, mgr.executionStore.Add(e))
			mgr.applyRecoveredTurnOutcome(context.Background(), e,
				&ExecutorInstance{RuntimeName: executor.NameStandalone, StandaloneInstanceID: "instance"}, outcome)
			at, _, _ := e.promptActivitySnapshot()
			if known {
				require.True(t, at.Equal(old))
			} else {
				require.True(t, at.IsZero())
			}
			require.Len(t, backend.ackedCalls, 1)
			require.Greater(t, backend.publishedCountsAtAck[0], 0, "apply must precede ACK")
			require.Equal(t, 1, countEventType(eventBus.PublishedEvents, events.AgentReady))
			live := outcome.Event
			live.Recovered, live.RecoveredActivityAt, live.ControlTurnID = false, nil, outcome.TurnID
			mgr.handleAgentEvent(e, live)
			after, _, _ := e.promptActivitySnapshot()
			require.True(t, after.Equal(at), "redelivery must not change activity")
			require.Equal(t, 1, countEventType(eventBus.PublishedEvents, events.AgentReady))
		})
	}
}

// @covers AC-EXECUTORS-IDLE-PARKING-001.11
func TestIdleActivityDurableReplayUsesJournalTimeWithoutChangingPayload(t *testing.T) {
	ctx := context.Background()
	old := time.Now().UTC().Add(-time.Hour)
	payload, err := json.Marshal(agentctl.AgentEvent{Type: "complete"})
	require.NoError(t, err)
	repository, database := runOwnerTestRepository(t)
	client, committed, stream, acknowledged := newIdleActivityJournalPeer(t, journal.Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
		StreamID: "stream", Type: "complete", Payload: payload, CreatedAt: old, Terminal: true,
	}, repository)
	status, err := client.GetDeliveryStatus(ctx, "stream")
	require.NoError(t, err, "positive control: real ACK owner descriptor route is available")
	require.True(t, status.Durable)
	require.Equal(t, "incarnation", status.IncarnationID)
	require.NoError(t, repository.CreateTask(ctx, &models.Task{ID: "task", Title: "Idle replay"}))
	require.NoError(t, repository.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session", TaskID: "task", QueueIncarnationID: "incarnation",
	}))
	require.NoError(t, repository.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: "session", IncarnationID: "incarnation", Generation: 2, NativeSessionID: "native", CreationReason: "initial",
	}))
	mgr := newTestManager(t)
	e := createTestExecution("execution", "task", "session")
	e.DeliveryMode, e.DeliveryStreamID, e.DeliveryIncarnationID, e.DeliveryHarnessGeneration = DurableDeliveryV1, "stream", "incarnation", 2
	e.DeliveryDescriptor, e.agentctl = &agentctl.DeliveryStatus{Stream: &stream}, client
	require.NoError(t, mgr.executionStore.Add(e))
	mgr.SetExecutorRunningWriter(repository)
	require.NoError(t, mgr.persistExecutorRunningResult(ctx, e))
	owner, err := repository.GetExecutorRunningBySessionID(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, e.ID, owner.AgentExecutionID, "positive control: real persistence installed the live delivery owner")
	original := agentctl.AgentEvent{Type: "complete", DeliveryStreamID: "stream", DeliverySequence: 1,
		DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2}
	expectedPayload := durableAgentEvent(e, original).Payload
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{OnAgentEvent: mgr.handleAgentEvent}, nil, nil)
	t.Cleanup(sm.Wait)
	sm.setAgentDeliveryRepository(repository)
	require.NoError(t, sm.ReplayRecoveredDelivery(ctx, e))
	at, _, _ := e.promptActivitySnapshot()
	require.True(t, at.Equal(old), "terminal lifecycle must use journal CreatedAt, never replay time")
	require.Equal(t, uint64(1), e.DeliveryReplayCursor)
	cursor, err := repository.GetAgentDeliveryCursor(ctx, "stream")
	require.NoError(t, err)
	require.EqualValues(t, 1, cursor.ProjectedSequence)
	var admitted []byte
	require.NoError(t, database.QueryRowContext(ctx, `SELECT payload FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?`, "stream", 1).Scan(&admitted))
	require.Equal(t, expectedPayload, admitted, "recovery context must not alter admitted payload identity")
	select {
	case sequence := <-acknowledged:
		require.Equal(t, uint64(1), sequence)
	case <-time.After(5 * time.Second):
		t.Fatal("projected terminal was not acknowledged")
	}
	require.NoError(t, sm.processRecoveredDeliveryEvent(ctx, e, client, repository, committed, 0, false))
	replayed, _, _ := e.promptActivitySnapshot()
	require.True(t, replayed.Equal(old))
	var effects int
	require.NoError(t, database.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_delivery_effects WHERE stream_id = ?`, "stream").Scan(&effects))
	require.Equal(t, 1, effects, "duplicate terminal must not apply twice")
}

func newIdleActivityJournalPeer(t *testing.T, source journal.Event, repository AgentDeliveryRepository) (*agentctl.Client, journal.Event, journal.Stream, <-chan uint64) {
	t.Helper()
	retained := openRetentionLifecycleJournal(t, filepath.Join(t.TempDir(), "idle.bbolt"))
	committed, err := retained.Append(t.Context(), source)
	require.NoError(t, err)
	stream, err := retained.GetStream(t.Context(), source.StreamID)
	require.NoError(t, err)
	acknowledged := make(chan uint64, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Query().Get("stream_id") != source.StreamID {
			http.Error(w, "unexpected delivery stream", http.StatusBadRequest)
			return
		}
		if r.URL.Path == "/api/v1/agent/delivery" {
			descriptor, err := retained.RecoveryDescriptor(r.Context(), source.SessionID, source.IncarnationID, source.HarnessGeneration, source.StreamID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(descriptor)
			return
		}
		if r.URL.Path == "/api/v1/agent/delivery/stream/ack" {
			var ack struct {
				StreamID string `json:"stream_id"`
				Sequence uint64 `json:"sequence"`
			}
			if err := json.NewDecoder(r.Body).Decode(&ack); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			cursor, err := repository.GetAgentDeliveryCursor(r.Context(), ack.StreamID)
			if err != nil || cursor == nil || cursor.ProjectedSequence < int64(ack.Sequence) {
				t.Errorf("ACK preceded SQL projection: cursor=%+v, err=%v", cursor, err)
				http.Error(w, "ACK preceded SQL projection", http.StatusConflict)
				return
			}
			if err := retained.Acknowledge(r.Context(), ack.StreamID, ack.Sequence); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			acknowledged <- ack.Sequence
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path != "/api/v1/agent/delivery/stream" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		after, err := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		page, state, err := retained.Replay(r.Context(), source.StreamID, after, 1000)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(struct {
			Events []journal.Event `json:"events"`
			Stream journal.Stream  `json:"stream"`
		}{page, state})
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)
	return agentctl.NewClient(parsed.Hostname(), port, newTestLogger()), committed, stream, acknowledged
}
