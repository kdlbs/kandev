package lifecycle

import (
	"context"
	"testing"
)

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.1
func TestMCPHandlerForUsesDispatcherInstalledAfterStreamCreation(t *testing.T) {
	log := newTestLogger()
	sm := NewStreamManager(log, StreamCallbacks{}, nil, nil)
	handler := sm.mcpHandlerFor(&AgentExecution{
		ID:        "exec-recovered",
		TaskID:    "task-recovered",
		SessionID: "session-recovered",
	})
	if handler == nil {
		t.Fatal("stream captured no MCP handler before dispatcher setup")
	}

	inner := &recordingMCPHandler{}
	manager := &Manager{streamManager: sm}
	manager.SetMCPHandler(inner)

	if _, err := handler.Dispatch(context.Background(), mcpRequest(t, nil)); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("dispatcher calls = %d, want 1", inner.calls)
	}
}
