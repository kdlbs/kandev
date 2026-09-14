package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDurableAdoptionReplayReadsEveryPageToCapturedHighWater(t *testing.T) {
	const highWater = 1001
	events := make([]journal.Event, 0, highWater)
	for sequence := uint64(1); sequence <= highWater; sequence++ {
		payload, err := json.Marshal(agentctl.AgentEvent{Type: "plan", Text: "replayed"})
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, journal.Event{
			SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
			StreamID: "stream-1", Sequence: sequence, Type: "plan", Payload: payload,
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/delivery/stream/ack" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path != "/api/v1/agent/delivery/stream" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		after, err := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		start := int(after)
		end := start + limit
		if end > len(events) {
			end = len(events)
		}
		_ = json.NewEncoder(w).Encode(struct {
			Events []journal.Event `json:"events"`
			Stream journal.Stream  `json:"stream"`
		}{
			Events: events[start:end],
			Stream: journal.Stream{
				SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
				StreamID: "stream-1", HighWater: highWater, FirstRetained: 1,
			},
		})
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	client := agentctl.NewClient(parsed.Hostname(), port, newTestLogger())
	repository := &recordingAgentDeliveryRepository{
		cursor: &models.AgentDeliveryCursor{StreamID: "stream-1"},
	}
	var callbackCount atomic.Int64
	streamManager := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) {
			if event.DeliverySequence == 0 {
				t.Errorf("replayed event did not carry its sequence")
			}
			callbackCount.Add(1)
		},
	}, nil, nil)
	streamManager.setAgentDeliveryRepository(repository)
	execution := &AgentExecution{
		SessionID:                 "session-1",
		DeliveryMode:              DurableDeliveryV1,
		DeliveryStreamID:          "stream-1",
		DeliveryIncarnationID:     "incarnation-1",
		DeliveryHarnessGeneration: 2,
		DeliveryReplayCursor:      0,
		DeliveryDescriptor: &agentctl.DeliveryStatus{
			Stream: &journal.Stream{
				SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
				StreamID: "stream-1", HighWater: highWater, FirstRetained: 1,
			},
		},
		agentctl: client,
	}

	if err := streamManager.ReplayRecoveredDelivery(context.Background(), execution); err != nil {
		t.Fatalf("ReplayRecoveredDelivery: %v", err)
	}
	if got := callbackCount.Load(); got != highWater {
		t.Fatalf("callback count = %d, want %d", got, highWater)
	}
	if execution.DeliveryReplayCursor != highWater {
		t.Fatalf("replay cursor = %d, want %d", execution.DeliveryReplayCursor, highWater)
	}
}

func TestDurableAdoptionReplayRejectsGapBeforeHighWater(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/delivery/stream/ack" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_ = json.NewEncoder(w).Encode(struct {
			Events []journal.Event `json:"events"`
			Stream journal.Stream  `json:"stream"`
		}{
			Events: nil,
			Stream: journal.Stream{
				SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
				StreamID: "stream-1", HighWater: 2, FirstRetained: 1,
			},
		})
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	client := agentctl.NewClient(parsed.Hostname(), port, newTestLogger())
	streamManager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	streamManager.setAgentDeliveryRepository(&recordingAgentDeliveryRepository{
		cursor: &models.AgentDeliveryCursor{StreamID: "stream-1"},
	})
	execution := &AgentExecution{
		SessionID:                 "session-1",
		DeliveryMode:              DurableDeliveryV1,
		DeliveryStreamID:          "stream-1",
		DeliveryIncarnationID:     "incarnation-1",
		DeliveryHarnessGeneration: 2,
		DeliveryDescriptor: &agentctl.DeliveryStatus{Stream: &journal.Stream{
			SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
			StreamID: "stream-1", HighWater: 2, FirstRetained: 1,
		}},
		agentctl: client,
	}

	if err := streamManager.ReplayRecoveredDelivery(context.Background(), execution); err == nil {
		t.Fatal("ReplayRecoveredDelivery succeeded across an empty page before high-water")
	}
}
