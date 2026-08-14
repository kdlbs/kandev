package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/mcp/plugintools"
)

func TestSetMcpProvidersForSessionCallsLiveAgentctl(t *testing.T) {
	var got []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/mcp/providers" {
			t.Errorf("request = %s %s, want PUT /api/v1/mcp/providers", r.Method, r.URL.Path)
		}
		var body struct {
			Providers []string `json:"mcp_providers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		got = body.Providers
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mgr, execution := workspaceSourceTestManager(t, server.URL, nil)
	providers := []string{"github", "gitlab"}
	if err := mgr.SetMcpProvidersForSession(context.Background(), execution.SessionID, providers); err != nil {
		t.Fatalf("SetMcpProvidersForSession: %v", err)
	}
	if !reflect.DeepEqual(got, providers) {
		t.Fatalf("agentctl providers = %v, want %v", got, providers)
	}
}

func TestSetMcpProvidersForSession_NoExecutionIsNoOp(t *testing.T) {
	mgr := &Manager{executionStore: NewExecutionStore(), logger: newTestLogger()}
	if err := mgr.SetMcpProvidersForSession(context.Background(), "session-missing", []string{"gitlab"}); err != nil {
		t.Fatalf("SetMcpProvidersForSession without execution: %v", err)
	}
}

func TestSetPluginToolsForAllExecutionsCallsLiveAgentctl(t *testing.T) {
	got := make(chan plugintools.Snapshot, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/mcp/plugin-tools" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		var snapshot plugintools.Snapshot
		if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
			t.Error(err)
		}
		got <- snapshot
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	mgr, _ := workspaceSourceTestManager(t, server.URL, nil)
	want := plugintools.Snapshot{Generation: "g", Revision: 3}
	if err := mgr.SetPluginToolsForAllExecutions(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	if snapshot := <-got; snapshot.Generation != want.Generation || snapshot.Revision != want.Revision {
		t.Fatalf("snapshot = %#v, want %#v", snapshot, want)
	}
}
