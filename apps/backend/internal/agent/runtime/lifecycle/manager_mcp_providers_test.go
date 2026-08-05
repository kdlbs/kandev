package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
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
