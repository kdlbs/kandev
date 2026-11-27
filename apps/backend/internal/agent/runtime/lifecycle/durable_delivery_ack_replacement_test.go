package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.4
func TestDurableAckReplacementUsesCurrentConnectionWithoutNewOutput(t *testing.T) {
	oldCalls := make(chan struct{}, 1)
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case oldCalls <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer old.Close()
	acknowledged := make(chan uint64, 1)
	current := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(journal.RecoveryDescriptor{
				StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
				SessionID:         "session", IncarnationID: "owner", HarnessGeneration: 1, StreamID: "stream",
				Stream: &journal.Stream{SessionID: "session", IncarnationID: "owner", HarnessGeneration: 1, StreamID: "stream", HighWater: 2074, Acknowledged: 1193},
			})
			return
		}
		var body struct {
			Sequence uint64 `json:"sequence"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		select {
		case acknowledged <- body.Sequence:
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer current.Close()
	clientFor := func(server *httptest.Server) *agentctl.Client {
		t.Helper()
		u, err := url.Parse(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		port, err := strconv.Atoi(u.Port())
		if err != nil {
			t.Fatal(err)
		}
		return agentctl.NewClient(u.Hostname(), port, newTestLogger())
	}
	first, replacement := clientFor(old), clientFor(current)
	execution := &AgentExecution{ID: "execution", SessionID: "session", DeliveryMode: DurableDeliveryV1,
		DeliveryStreamID: "stream", DeliveryIncarnationID: "owner", DeliveryHarnessGeneration: 1, agentctl: first}
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	defer sm.Wait()
	sm.scheduleDurableDeliveryAck(execution, first, agentctl.AgentEvent{DeliveryStreamID: "stream", DeliverySequence: 2074})
	select {
	case <-oldCalls:
	case <-time.After(3 * time.Second):
		t.Fatal("old request did not start")
	}
	execution.replaceAgentctlClient(replacement)
	// No further event or prompt: the pending worker must follow the execution's lease.
	select {
	case sequence := <-acknowledged:
		if sequence != 2074 {
			t.Fatalf("ACK = %d", sequence)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("replacement did not receive pending ACK")
	}
}

func TestQuietRecoveredStreamAcknowledgesProjectedCursor(t *testing.T) {
	ack := make(chan uint64, 1)
	stream := journal.Stream{SessionID: "session", IncarnationID: "owner", HarnessGeneration: 1, StreamID: "stream", HighWater: 2074, Acknowledged: 1193}
	client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(journal.RecoveryDescriptor{StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true}, SessionID: "session", IncarnationID: "owner", HarnessGeneration: 1, StreamID: "stream", Stream: &stream})
			return
		}
		var request struct {
			Sequence uint64 `json:"sequence"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		ack <- request.Sequence
		w.WriteHeader(http.StatusNoContent)
	})
	execution := &AgentExecution{ID: "execution", SessionID: "session", DeliveryMode: DurableDeliveryV1, DeliveryStreamID: "stream", DeliveryIncarnationID: "owner", DeliveryHarnessGeneration: 1, DeliveryReplayCursor: 2074, DeliveryDescriptor: &agentctl.DeliveryStatus{Stream: &stream}, agentctl: client}
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	defer sm.Wait()
	sm.setAgentDeliveryRepository(&recordingAgentDeliveryRepository{cursor: &models.AgentDeliveryCursor{StreamID: "stream", ReceivedSequence: 2074, ProjectedSequence: 2074}})
	if err := sm.ReplayRecoveredDelivery(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	select {
	case sequence := <-ack:
		if sequence != 2074 {
			t.Fatalf("ACK = %d", sequence)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("quiet replay failed to reconcile ACK")
	}
}

func TestDeliveryAckRejectsForeignOwner(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(strconv.FormatBool(foreign), func(t *testing.T) {
			var acked atomic.Bool
			client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					acked.Store(true)
					w.WriteHeader(http.StatusNoContent)
					return
				}
				owner := "owner"
				if foreign {
					owner = "replacement-owner"
				}
				_ = json.NewEncoder(w).Encode(journal.RecoveryDescriptor{StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true}, SessionID: "session", IncarnationID: owner, HarnessGeneration: 1, StreamID: "stream", Stream: &journal.Stream{SessionID: "session", IncarnationID: owner, HarnessGeneration: 1, StreamID: "stream", HighWater: 3}})
			})
			execution := &AgentExecution{SessionID: "session", DeliveryIncarnationID: "owner", DeliveryHarnessGeneration: 1, agentctl: client}
			sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			defer sm.Wait()
			err := sm.deliveryAckSender(execution, "stream")(context.Background(), 3)
			if foreign {
				if !errors.Is(err, journal.ErrOwnerMismatch) || acked.Load() {
					t.Fatalf("foreign owner ACK: %v, sent %v", err, acked.Load())
				}
			} else if err != nil || !acked.Load() {
				t.Fatalf("valid owner ACK: %v, sent %v", err, acked.Load())
			}
		})
	}
}

func ackTestClient(t *testing.T, handler http.HandlerFunc) *agentctl.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return agentctl.NewClient(u.Hostname(), port, newTestLogger())
}

func TestDeliveryAckOwnerMismatchStopsQuietWorker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var attempts atomic.Int32
		worker := newDurableDeliveryAckWorker(func(context.Context, uint64) error {
			attempts.Add(1)
			return journal.ErrOwnerMismatch
		})
		defer worker.cancel()
		done := make(chan struct{})
		go func() { defer close(done); worker.run() }()
		worker.schedule(3, true)
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("owner mismatch left an obsolete ACK worker retrying")
		}
		if attempts.Load() != 1 {
			t.Fatalf("ACK attempts = %d", attempts.Load())
		}
	})
}
