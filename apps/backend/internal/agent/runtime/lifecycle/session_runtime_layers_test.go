package lifecycle

import (
	"context"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// settledCatalogState builds a CachedModelState whose config-option catalog is
// settled and advertises exactly the given option IDs (category left blank so
// they are restorable), each with a current value.
func settledCatalogState(currentModel string, advertised map[string]string) *CachedModelState {
	options := make([]streams.ConfigOption, 0, len(advertised))
	for id, value := range advertised {
		options = append(options, streams.ConfigOption{
			Type:         "select",
			ID:           id,
			CurrentValue: value,
		})
	}
	return &CachedModelState{
		CurrentModelID:       currentModel,
		ConfigOptions:        options,
		ConfigOptionsSettled: true,
	}
}

// configOptionRecorder wraps the mock handler to capture the config_id values
// that reach the agent via set_config_option, and optionally errors on one id.
func configOptionRecorder(mock *mockAgentServer, failID string) *sync.Map {
	seen := &sync.Map{}
	base := mock.defaultHandler
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.session.set_config_option" {
			var payload struct {
				ConfigID string `json:"config_id"`
				Value    string `json:"value"`
			}
			if err := msg.ParsePayload(&payload); err == nil {
				seen.Store(payload.ConfigID, payload.Value)
				if failID != "" && payload.ConfigID == failID {
					resp, _ := ws.NewError(msg.ID, msg.Action,
						ws.ErrorCodeInternalError, "option not supported", nil)
					return resp
				}
			}
		}
		return base(msg)
	}
	return seen
}

// connectAgentStream opens the agent updates WebSocket so SetConfigOption calls
// can round-trip, then waits until the mock observes the connection.
func connectAgentStream(t *testing.T, mock *mockAgentServer, client *agentctl.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	if err := client.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("connect agent stream: %v", err)
	}
	select {
	case <-mock.wsConnected:
	case <-time.After(5 * time.Second):
		t.Fatal("agent stream never connected")
	}
}

// TestApplyRuntimeSessionLayersFiltersUnadvertisedOptions verifies that the
// startup/resume replay path drops persisted config options the current agent
// does not advertise, so they are neither sent nor reported as startup
// failures, while advertised options still apply and real failures surface.
func TestApplyRuntimeSessionLayersFiltersUnadvertisedOptions(t *testing.T) {
	sentKeys := func(seen *sync.Map) []string {
		keys := make([]string, 0)
		seen.Range(func(k, _ any) bool {
			keys = append(keys, k.(string))
			return true
		})
		sort.Strings(keys)
		return keys
	}

	t.Run("drops unadvertised prior-agent keys", func(t *testing.T) {
		mock := newMockAgentServer(t)
		defer mock.Close()
		seen := configOptionRecorder(mock, "")

		sm := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
		client := createTestClient(t, mock.server.URL)
		defer client.Close()
		connectAgentStream(t, mock, client)
		execution := &AgentExecution{ID: "e1", SessionID: "s1", agentctl: client}
		execution.SetModelState(settledCatalogState("cur", map[string]string{
			"reasoning": "high", "fast": "on", "context": "repo",
		}))

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, failed := sm.applyRuntimeSessionLayers(
			ctx, execution, "acp-1", "cfg", "", "", "", "",
			map[string]string{"reasoning": "high", "fast": "on", "effort": "x", "thinking": "y"},
		)

		if len(failed) != 0 {
			t.Fatalf("expected no failed options, got %v", failed)
		}
		got := sentKeys(seen)
		want := []string{"fast", "reasoning"}
		if !slices.Equal(got, want) {
			t.Fatalf("sent config options = %v, want %v (effort/thinking must be dropped)", got, want)
		}
	})

	t.Run("unknown catalog (nil model state) sends no options and reports no failures", func(t *testing.T) {
		mock := newMockAgentServer(t)
		defer mock.Close()
		seen := configOptionRecorder(mock, "")

		sm := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
		client := createTestClient(t, mock.server.URL)
		defer client.Close()
		connectAgentStream(t, mock, client)
		// Leave model state nil: the current agent's option catalog is unknown,
		// so replay must fail safe and send nothing (spec failure mode).
		execution := &AgentExecution{ID: "e3", SessionID: "s3", agentctl: client}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, failed := sm.applyRuntimeSessionLayers(
			ctx, execution, "acp-3", "cfg", "", "", "", "",
			map[string]string{"reasoning": "high", "effort": "x", "thinking": "y"},
		)

		if len(failed) != 0 {
			t.Fatalf("expected no failed options for unknown catalog, got %v", failed)
		}
		if got := sentKeys(seen); len(got) != 0 {
			t.Fatalf("expected no options sent for unknown catalog, got %v", got)
		}
	})

	t.Run("advertised option value rejection still surfaces", func(t *testing.T) {
		mock := newMockAgentServer(t)
		defer mock.Close()
		configOptionRecorder(mock, "reasoning")

		sm := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
		client := createTestClient(t, mock.server.URL)
		defer client.Close()
		connectAgentStream(t, mock, client)
		execution := &AgentExecution{ID: "e2", SessionID: "s2", agentctl: client}
		execution.SetModelState(settledCatalogState("cur", map[string]string{
			"reasoning": "high", "fast": "on",
		}))

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, failed := sm.applyRuntimeSessionLayers(
			ctx, execution, "acp-2", "cfg", "", "", "", "",
			map[string]string{"reasoning": "high", "effort": "x"},
		)

		if len(failed) != 1 || failed[0] != "reasoning" {
			t.Fatalf("expected [reasoning] in failed, got %v", failed)
		}
	})
}
