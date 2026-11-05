package codexappserver

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/common/logger"
)

// sandboxByMethod runs NewSession then LoadSession against a fake app-server
// and returns the sandbox mode each thread call carried.
func sandboxByMethod(t *testing.T, cfg *shared.Config) map[string]string {
	t.Helper()
	var mu sync.Mutex
	seen := map[string]string{}
	server := newProtocolServer(t, func(req map[string]json.RawMessage, write func(any) error) error {
		method := readString(req, "method")
		id := req["id"]
		switch method {
		case "initialize":
			return write(resultFrame(id, map[string]any{"userAgent": "Codex CLI 0.154.0"}))
		case "initialized":
			return nil
		case "model/list":
			return write(resultFrame(id, map[string]any{"data": []any{}}))
		case "thread/start", "thread/resume":
			var params struct {
				Sandbox string `json:"sandbox"`
			}
			if err := json.Unmarshal(req["params"], &params); err != nil {
				return err
			}
			mu.Lock()
			seen[method] = params.Sandbox
			mu.Unlock()
			return write(resultFrame(id, map[string]any{"thread": map[string]any{"id": "thread-1"}, "model": "gpt-5"}))
		default:
			return write(errorFrame(id, -32601, "unsupported"))
		}
	})
	defer server.close()

	adapter := NewAdapter(cfg, logger.Default())
	defer func() { _ = adapter.Close() }()
	if err := adapter.Connect(server.clientWriter, server.clientReader); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := adapter.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if _, err := adapter.NewSession(ctx, nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := adapter.LoadSession(ctx, "thread-1", nil); err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	return map[string]string{"thread/start": seen["thread/start"], "thread/resume": seen["thread/resume"]}
}

func TestSandboxModeFollowsEnforcedNodePolicy(t *testing.T) {
	tests := []struct {
		name     string
		enforced bool
		want     string
	}{
		{"default keeps workspace-write", false, "workspace-write"},
		{"enforced node policy drops Codex's inner sandbox", true, "danger-full-access"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sandboxByMethod(t, &shared.Config{WorkDir: "/workspace", OuterSandboxEnforced: tt.enforced})
			for _, method := range []string{"thread/start", "thread/resume"} {
				if got[method] != tt.want {
					t.Errorf("%s sandbox = %q, want %q", method, got[method], tt.want)
				}
			}
		})
	}
}
