package lifecycle

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

func TestDurableDeliveryCapabilityRefreshesTransientRecoveryState(t *testing.T) {
	var unresolved atomic.Bool
	unresolved.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/agent/delivery" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		reason := ""
		if unresolved.Load() {
			reason = "unresolved_durable_work"
		}
		_ = json.NewEncoder(w).Encode(journal.RecoveryDescriptor{
			StorageCapability: journal.StorageCapability{
				Version:    journal.CurrentVersion,
				Durable:    true,
				Unresolved: unresolved.Load(),
				Reason:     reason,
			},
			SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 1,
			StreamID: "stream-1",
		})
	}))
	t.Cleanup(server.Close)
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	client := agentctl.NewClient(host, port, newTestLogger())
	mgr := newTestManager(t)
	if err := mgr.executionStore.Add(&AgentExecution{
		ID: "execution-1", SessionID: "session-1", DeliveryStreamID: "stream-1", agentctl: client,
	}); err != nil {
		t.Fatal(err)
	}

	first, advertised := mgr.DurableDeliveryCapabilityForExecution(context.Background(), "execution-1")
	if !advertised || !first.Unresolved || first.Reason != "unresolved_durable_work" {
		t.Fatalf("first capability = %+v, advertised=%t", first, advertised)
	}
	unresolved.Store(false)
	second, advertised := mgr.DurableDeliveryCapabilityForExecution(context.Background(), "execution-1")
	if !advertised || second.Unresolved || second.Reason != "" {
		t.Fatalf("refreshed capability = %+v, advertised=%t", second, advertised)
	}
}
