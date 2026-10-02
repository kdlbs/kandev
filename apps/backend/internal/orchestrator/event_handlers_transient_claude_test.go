package orchestrator

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

const claudeSessionLimitNotice = "Internal error: You've hit your session limit · resets 11:10am (Europe/Helsinki)"

// @covers AC-AGENTS-CLAUDE-SESSION-LIMIT-001.1, AC-AGENTS-CLAUDE-SESSION-LIMIT-001.2
func TestClassifyKanbanFailureClaudeSessionLimit(t *testing.T) {
	occurredAt := time.Date(2024, time.January, 1, 8, 3, 0, 0, time.UTC)
	classified := classifyKanbanFailure(watcher.AgentEventData{
		AgentID:      "claude-acp",
		ErrorMessage: claudeSessionLimitNotice,
		ProviderError: &streams.ProviderError{
			Source:     streams.ProviderErrorSourceACPPrompt,
			ProviderID: "claude-acp",
			Message:    claudeSessionLimitNotice,
			OccurredAt: occurredAt,
		},
	})
	if classified.Code != routingerr.CodeQuotaLimited || classified.Confidence != routingerr.ConfHigh || classified.Class != routingerr.ClassHard {
		t.Fatalf("classification = %+v, want high-confidence hard quota_limited", classified)
	}
	if classified.ResetHint == nil || !classified.FallbackAllowed {
		t.Fatalf("classification = %+v, want a reset hint and fallback eligibility", classified)
	}
	wantReset := time.Date(2024, time.January, 1, 9, 10, 0, 0, time.UTC)
	if !classified.ResetHint.Equal(wantReset) {
		t.Fatalf("reset hint = %s, want occurrence-anchored %s", classified.ResetHint, wantReset)
	}
}

// @covers AC-AGENTS-CLAUDE-SESSION-LIMIT-001.6
func TestClaudeQuotaFixedProfileRemainsManual(t *testing.T) {
	now := time.Date(2026, time.October, 1, 8, 3, 0, 0, time.UTC)
	resetAt := now.Add(24 * time.Hour)
	classified := classifyKanbanFailure(watcher.AgentEventData{
		AgentID:      "claude-acp",
		ErrorMessage: claudeSessionLimitNotice,
		ProviderError: &streams.ProviderError{
			Source:     streams.ProviderErrorSourceACPPrompt,
			ProviderID: "claude-acp",
			Message:    claudeSessionLimitNotice,
			OccurredAt: now,
			ResetAt:    &resetAt,
		},
	})
	if got := routingerr.Decide(routingerr.ContextKanban, classified, now); got != routingerr.DecisionManual {
		t.Fatalf("fixed-profile recovery decision = %q, want manual", got)
	}
}
