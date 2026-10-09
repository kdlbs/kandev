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

func TestRetainedDeliveryProjectionDoesNotInvokeLifecycleCallbacks(t *testing.T) {
	const (
		sessionID     = "session-retained-projection"
		incarnationID = "incarnation-retained-projection"
		streamID      = "stream-retained-projection"
		submissionID  = "submission-retained-projection"
	)
	payload, err := json.Marshal(agentctl.AgentEvent{
		Type: "complete", SessionID: sessionID, TurnID: "turn-retained-projection",
		DeliveryStreamID: streamID, DeliverySequence: 1, DeliverySubmissionID: submissionID,
		DeliveryIncarnationID: incarnationID, DeliveryHarnessGeneration: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	event := journal.Event{
		SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 2,
		StreamID: streamID, Sequence: 1, SubmissionID: submissionID,
		Type: "complete", Payload: payload, Terminal: true,
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
			SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 2,
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
	repository := &recordingAgentDeliveryRepository{cursor: &models.AgentDeliveryCursor{StreamID: streamID}}
	streamManager := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(*AgentExecution, agentctl.AgentEvent) { callbackCount.Add(1) },
	}, nil, nil)
	t.Cleanup(streamManager.Wait)
	streamManager.setAgentDeliveryRepository(repository)
	execution := &AgentExecution{
		ID: "execution-retained-projection", TaskID: "task-retained-projection", SessionID: sessionID,
		DeliveryMode: DurableDeliveryV1, DeliveryStreamID: streamID,
		DeliveryIncarnationID: incarnationID, DeliveryHarnessGeneration: 2,
		DeliveryDescriptor: &agentctl.DeliveryStatus{Stream: &journal.Stream{
			SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 2,
			StreamID: streamID, HighWater: 1, FirstRetained: 1,
		}}, agentctl: client,
	}

	if err := streamManager.ReplayRecoveredDeliveryWithoutLifecycleCallbacks(context.Background(), execution); err != nil {
		t.Fatalf("replay retained delivery: %v", err)
	}
	if got := callbackCount.Load(); got != 0 {
		t.Fatalf("lifecycle callbacks = %d, want none during evidence projection", got)
	}
	if len(repository.projected) != 1 || repository.projected[0].Sequence != 1 {
		t.Fatalf("projected events = %+v, want retained terminal event", repository.projected)
	}
}
