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
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

// TestCopilotCompactionProjectionOnlyRecovery proves a journaled native
// completion is projected through the canonical path with no lifecycle
// callback. The authoritative SQL increment is covered by the repository
// projection tests; this test pins the recovery routing that must reach it.
func TestCopilotCompactionProjectionOnlyRecovery(t *testing.T) {
	const (
		sessionID     = "session-copilot-compaction-recovery"
		incarnationID = "incarnation-copilot-compaction-recovery"
		streamID      = "stream-copilot-compaction-recovery"
		submissionID  = "submission-copilot-compaction-recovery"
	)
	observation := &streams.ContextCompactionObservation{
		Kind:            streams.ContextCompactionObservationKindCompletion,
		Provider:        "copilot-acp",
		NativeSessionID: "native-copilot-compaction",
		AttemptID:       "attempt-recovery",
		Success:         true,
		Outcome:         "applied",
	}
	payload, err := json.Marshal(agentctl.AgentEvent{
		Type:              streams.EventTypeContextCompactionObservation,
		SessionID:         "native-copilot-compaction",
		ContextCompaction: observation,
		DeliveryStreamID:  streamID, DeliverySequence: 1, DeliverySubmissionID: submissionID,
		DeliveryIncarnationID: incarnationID, DeliveryHarnessGeneration: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	event := journal.Event{
		SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 7,
		StreamID: streamID, Sequence: 1, SubmissionID: submissionID,
		Type: streams.EventTypeContextCompactionObservation, Payload: payload,
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
		_ = json.NewEncoder(w).Encode(struct {
			Events []journal.Event `json:"events"`
			Stream journal.Stream  `json:"stream"`
		}{Events: []journal.Event{event}, Stream: journal.Stream{
			SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 7,
			StreamID: streamID, HighWater: 1, FirstRetained: 1,
		}})
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
	var callbackCount atomic.Int64
	repository := &canonicalProjectionDeliveryRepository{
		recordingAgentDeliveryRepository: recordingAgentDeliveryRepository{
			cursor: &models.AgentDeliveryCursor{StreamID: streamID},
		},
	}
	streamManager := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(*AgentExecution, agentctl.AgentEvent) { callbackCount.Add(1) },
	}, nil, nil)
	t.Cleanup(streamManager.Wait)
	streamManager.setAgentDeliveryRepository(repository)
	execution := &AgentExecution{
		ID: "execution-copilot-compaction-recovery", TaskID: "task-copilot-compaction-recovery",
		SessionID: sessionID, DeliveryMode: DurableDeliveryV1, DeliveryStreamID: streamID,
		DeliveryIncarnationID: incarnationID, DeliveryHarnessGeneration: 7,
		DeliveryDescriptor: &agentctl.DeliveryStatus{Stream: &journal.Stream{
			SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 7,
			StreamID: streamID, HighWater: 1, FirstRetained: 1,
		}}, agentctl: client,
	}

	if err := streamManager.ReplayRecoveredDeliveryWithoutLifecycleCallbacks(context.Background(), execution); err != nil {
		t.Fatalf("replay retained delivery: %v", err)
	}
	if got := callbackCount.Load(); got != 0 {
		t.Fatalf("lifecycle callbacks = %d, want none during projection-only recovery", got)
	}
	if len(repository.projected) != 1 || repository.projected[0].EventType != streams.EventTypeContextCompactionObservation {
		t.Fatalf("projected events = %+v, want the journaled native observation", repository.projected)
	}
}

func TestCanonicalAgentDeliveryEventRecognizesCompactionObservation(t *testing.T) {
	if !canonicalAgentDeliveryEvent(agentctl.AgentEvent{Type: streams.EventTypeContextCompactionObservation}) {
		t.Fatal("native compaction observation must use canonical projection")
	}
	if canonicalAgentDeliveryEvent(agentctl.AgentEvent{Type: "unknown_native_event"}) {
		t.Fatal("unknown provider event must stay outside canonical projection")
	}
}
