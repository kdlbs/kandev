package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuiltinMCPInstanceNegotiationResponse(t *testing.T) {
	for _, test := range []struct {
		name       string
		response   string
		wantScoped bool
		wantError  bool
	}{
		{name: "accepted scoped", response: `{"id":"exec-1","port":43120,"scoped_mcp_routes":true}`, wantScoped: true},
		{name: "accepted legacy", response: `{"id":"exec-1","port":43120,"scoped_mcp_routes":false}`},
		{name: "old helper omits field", response: `{"id":"exec-1","port":43120}`},
		{name: "malformed field", response: `{"id":"exec-1","port":43120,"scoped_mcp_routes":"true"}`, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read create body: %v", err)
				}
				var request CreateInstanceRequest
				if err := json.Unmarshal(body, &request); err != nil {
					t.Errorf("decode create body: %v", err)
				}
				if !request.ScopedMCPRoutes {
					t.Error("create request omitted scoped_mcp_routes=true")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(test.response))
			}))
			t.Cleanup(server.Close)

			response, err := newTestControlClient(t, server).CreateInstance(context.Background(), &CreateInstanceRequest{WorkspacePath: "/workspace", ScopedMCPRoutes: true})
			if test.wantError {
				if err == nil {
					t.Fatal("CreateInstance() error = nil, want malformed field error")
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateInstance() error = %v", err)
			}
			if response.ScopedMCPRoutes != test.wantScoped {
				t.Fatalf("ScopedMCPRoutes = %t, want %t", response.ScopedMCPRoutes, test.wantScoped)
			}
		})
	}
}

func TestBuiltinMCPInstanceInfoCompatibility(t *testing.T) {
	for _, test := range []struct {
		name       string
		json       string
		wantScoped bool
	}{
		{name: "old helper omits field", json: `{"id":"exec-1","port":43120}`},
		{name: "retained scoped helper", json: `{"id":"exec-1","port":43120,"scoped_mcp_routes":true}`, wantScoped: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var info InstanceInfo
			if err := json.Unmarshal([]byte(test.json), &info); err != nil {
				t.Fatalf("Unmarshal(InstanceInfo) error = %v", err)
			}
			if info.ScopedMCPRoutes != test.wantScoped {
				t.Fatalf("ScopedMCPRoutes = %t, want %t", info.ScopedMCPRoutes, test.wantScoped)
			}
		})
	}
}

func TestBuiltinMCPInstanceClientMode(t *testing.T) {
	legacy := NewClient("127.0.0.1", 1, newTestLogger(),
		WithExecutionID("helper-legacy"), WithBuiltinMCPScopedRoutes(false))
	scoped := NewClient("127.0.0.1", 1, newTestLogger(),
		WithExecutionID("helper-scoped"), WithBuiltinMCPScopedRoutes(true))

	if legacy.BuiltinMCPScopedRoutes() {
		t.Fatal("legacy client reports scoped routes")
	}
	if !scoped.BuiltinMCPScopedRoutes() {
		t.Fatal("scoped client lost accepted route mode")
	}
	if scoped.InstanceID() != "helper-scoped" {
		t.Fatalf("InstanceID() = %q, want accepted helper identity", scoped.InstanceID())
	}
}
