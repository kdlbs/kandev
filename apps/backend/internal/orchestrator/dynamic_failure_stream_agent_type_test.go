package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

func TestStreamFailureClassifiesByTransportType(t *testing.T) {
	const transportType = "opencode-acp"
	payload := &lifecycle.AgentStreamEventPayload{AgentID: "execution-1", ExecutionID: "execution-1", AgentType: transportType}
	if got := streamFailureAgentType(payload); got != transportType {
		t.Fatalf("agent type = %q, want the transport type", got)
	}
	classified := classifyKanbanFailure(watcher.AgentEventData{
		AgentID:      streamFailureAgentType(payload),
		ErrorMessage: "AI_APICallError: 5-hour usage limit reached. Resets in 4hr 19min.",
	})
	if classified.Code != routingerr.CodeQuotaLimited {
		t.Fatalf("classification = %+v, want quota_limited from the OpenCode rules", classified)
	}
	if got := streamFailureAgentType(&lifecycle.AgentStreamEventPayload{AgentID: "execution-2"}); got != "execution-2" {
		t.Fatalf("agent type without transport type = %q, want the stream AgentID", got)
	}
}
