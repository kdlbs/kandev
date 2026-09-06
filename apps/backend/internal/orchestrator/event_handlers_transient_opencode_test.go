package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// OpenCode stderr diagnostics carry the model-provider ID ("opencode-go"),
// not the agent ID the provider rules are keyed by. The Kanban classifier
// must keep the agent ID so a usage-limit error is still quota_limited and
// dynamic routing can advance past the exhausted candidate.
func TestClassifyKanbanFailure_OpenCodeModelProviderIDKeepsAgentRules(t *testing.T) {
	classified := classifyKanbanFailure(watcher.AgentEventData{
		AgentID:      "opencode-acp",
		ErrorMessage: "AI_APICallError: Weekly usage limit reached. Resets in 3 days.",
		ProviderError: &streams.ProviderError{
			Source:     streams.ProviderErrorSourceOpenCodeStderr,
			ProviderID: "opencode-go",
			ModelID:    "deepseek-v4-flash",
			Message:    "AI_APICallError: Weekly usage limit reached. Resets in 3 days.",
		},
	})
	if classified.Code != routingerr.CodeQuotaLimited || classified.Confidence != routingerr.ConfHigh {
		t.Fatalf("classification = %+v, want high-confidence quota_limited", classified)
	}
	if !classified.FallbackAllowed {
		t.Fatalf("quota_limited must allow dynamic fallback: %+v", classified)
	}
}

func TestClassifyKanbanFailure_OpenCodeModelProviderCollisionKeepsAgentRules(t *testing.T) {
	classified := classifyKanbanFailure(watcher.AgentEventData{
		AgentID:      "opencode-acp",
		ErrorMessage: "AI_APICallError: Weekly usage limit reached. Resets in 3 days.",
		ProviderError: &streams.ProviderError{
			Source:     streams.ProviderErrorSourceOpenCodeStderr,
			ProviderID: "claude-acp",
			Message:    "AI_APICallError: Weekly usage limit reached. Resets in 3 days.",
		},
	})
	if classified == nil {
		t.Fatal("classification = nil, want high-confidence quota_limited")
	}
	if classified.Code != routingerr.CodeQuotaLimited || classified.Confidence != routingerr.ConfHigh {
		t.Fatalf("classification = %+v, want high-confidence quota_limited", classified)
	}
	if !classified.FallbackAllowed {
		t.Fatalf("quota_limited must allow dynamic fallback: %+v", classified)
	}
}
