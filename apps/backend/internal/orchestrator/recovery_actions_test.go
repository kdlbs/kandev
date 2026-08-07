package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// thinkingBlocks400 is the resume-corrupted signature surfaced by the
// claude-agent-acp adapter after a session/load resume.
const thinkingBlocks400 = `{"code":-32603,"message":"Internal error: API Error: 400 messages.0.content.1: ` +
	"`thinking`" + ` or ` + "`redacted_thinking`" + ` blocks in the latest assistant message cannot be modified."}`

func actionByTestID(actions []map[string]interface{}, testID string) map[string]interface{} {
	for _, a := range actions {
		if a["test_id"] == testID {
			return a
		}
	}
	return nil
}

func TestBuildRecoveryActions_NormalOrdering(t *testing.T) {
	// Regression: ordinary failures keep Resume first, then Start fresh.
	actions := buildRecoveryActions("t1", "s1", true /*hasResumeToken*/, false /*auth*/, false /*resumeCorrupted*/)
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	if actions[0]["test_id"] != recoveryResumeButtonTestID {
		t.Errorf("expected resume button first, got %v", actions[0]["test_id"])
	}
	if actions[1]["test_id"] != recoveryFreshButtonTestID {
		t.Errorf("expected fresh button second, got %v", actions[1]["test_id"])
	}
}

func TestBuildRecoveryActions_ResumeCorruptedReordersFreshFirst(t *testing.T) {
	actions := buildRecoveryActions("t1", "s1", true /*hasResumeToken*/, false /*auth*/, true /*resumeCorrupted*/)
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions (fresh primary + resume), got %d", len(actions))
	}
	// Start fresh is the primary, so it comes first.
	if actions[0]["test_id"] != recoveryFreshButtonTestID {
		t.Errorf("expected fresh button first, got %v", actions[0]["test_id"])
	}
	// Resume is kept but de-emphasized with a note that it will likely fail.
	resume := actionByTestID(actions, recoveryResumeButtonTestID)
	if resume == nil {
		t.Fatal("expected resume button to still be present")
	}
	tooltip, _ := resume["tooltip"].(string)
	if !strings.Contains(strings.ToLower(tooltip), "likely fail") {
		t.Errorf("expected resume tooltip to warn it will likely fail, got %q", tooltip)
	}
}

func TestBuildRecoveryActions_ResumeCorruptedWithoutToken(t *testing.T) {
	// No resume token → only the fresh-start action regardless.
	actions := buildRecoveryActions("t1", "s1", false, false, true)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0]["test_id"] != recoveryFreshButtonTestID {
		t.Errorf("expected fresh button, got %v", actions[0]["test_id"])
	}
}

func TestCreateRecoveryStatusMessage_ResumeCorrupted(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	mc := &mockMessageCreator{}
	svc.messageCreator = mc

	svc.createRecoveryStatusMessage(ctx, watcher.AgentEventData{
		TaskID:       "t1",
		SessionID:    "s1",
		ErrorMessage: thinkingBlocks400,
	})

	if len(mc.sessionMessages) != 1 {
		t.Fatalf("expected 1 session message, got %d", len(mc.sessionMessages))
	}
	msg := mc.sessionMessages[0]
	if !strings.Contains(strings.ToLower(msg.content), "fresh session") {
		t.Errorf("expected message to steer toward a fresh session, got %q", msg.content)
	}
	if msg.metadata["resume_corrupted"] != true {
		t.Errorf("expected resume_corrupted=true in metadata, got %v", msg.metadata["resume_corrupted"])
	}
}

func TestCreateRecoveryStatusMessage_OpenCodeQuotaCarriesSafeMetadata(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-quota", "s-quota", "step1")
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	mc := &mockMessageCreator{}
	svc.messageCreator = mc
	resetAt := time.Date(2026, 8, 2, 19, 34, 44, 0, time.UTC)
	const wantURL = "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go"

	svc.createRecoveryStatusMessage(ctx, watcher.AgentEventData{
		TaskID:       "t-quota",
		SessionID:    "s-quota",
		AgentID:      "opencode-acp",
		ErrorMessage: "5-hour usage limit reached",
		ProviderError: &streams.ProviderError{
			Source:         streams.ProviderErrorSourceOpenCodeStderr,
			ProviderID:     "opencode-go",
			ModelID:        "kimi-k3",
			Message:        "5-hour usage limit reached",
			RemediationURL: wantURL,
			OccurredAt:     time.Date(2026, 8, 2, 15, 15, 44, 0, time.UTC),
			ResetAt:        &resetAt,
		},
	})

	if len(mc.sessionMessages) != 1 {
		t.Fatalf("expected 1 session message, got %d", len(mc.sessionMessages))
	}
	meta := mc.sessionMessages[0].metadata
	if meta["failure_kind"] != "provider_quota_limited" {
		t.Fatalf("failure_kind = %#v", meta["failure_kind"])
	}
	if meta["provider_name"] != "OpenCode" || meta["model_id"] != "kimi-k3" {
		t.Fatalf("provider metadata = %#v", meta)
	}
	if meta["reset_at"] != resetAt.Format(time.RFC3339) {
		t.Fatalf("reset_at = %#v", meta["reset_at"])
	}
	if meta["error_output"] != "5-hour usage limit reached" {
		t.Fatalf("error_output = %#v", meta["error_output"])
	}
	if meta["remediation_url"] != wantURL {
		t.Fatalf("remediation_url = %#v, want %q", meta["remediation_url"], wantURL)
	}
}

func TestCreateRecoveryStatusMessage_GenericFailureCarriesRemediationURL(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-generic", "s-generic", "step1")
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	mc := &mockMessageCreator{}
	svc.messageCreator = mc
	const wantURL = "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go"

	// A structured ACP-sourced diagnostic is not quota-classified, but the
	// validated link still reaches the generic recoverable card.
	svc.createRecoveryStatusMessage(ctx, watcher.AgentEventData{
		TaskID:       "t-generic",
		SessionID:    "s-generic",
		AgentID:      "opencode-acp",
		ErrorMessage: "AI_APICallError: usage limit reached",
		ProviderError: &streams.ProviderError{
			Source:         streams.ProviderErrorSourceOpenCodeACP,
			Message:        "usage limit reached",
			RemediationURL: wantURL,
			OccurredAt:     time.Date(2026, 8, 2, 15, 15, 44, 0, time.UTC),
		},
	})

	if len(mc.sessionMessages) != 1 {
		t.Fatalf("expected 1 session message, got %d", len(mc.sessionMessages))
	}
	meta := mc.sessionMessages[0].metadata
	if meta["failure_kind"] != nil {
		t.Fatalf("failure_kind = %#v, want unset for generic failure", meta["failure_kind"])
	}
	if meta["remediation_url"] != wantURL {
		t.Fatalf("remediation_url = %#v, want %q", meta["remediation_url"], wantURL)
	}
	if _, ok := meta["error_output"]; ok {
		t.Fatalf("error_output unexpectedly set for non-quota diagnostic: %#v", meta["error_output"])
	}
}

func TestProviderRemediationURLRejectsInvalidDiagnostics(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-none", "s-none", "step1")
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	mc := &mockMessageCreator{}
	svc.messageCreator = mc

	for _, tt := range []struct {
		name        string
		providerErr *streams.ProviderError
	}{
		{name: "no provider error"},
		{name: "no remediation url", providerErr: &streams.ProviderError{
			Source:     streams.ProviderErrorSourceOpenCodeStderr,
			Message:    "usage limit reached",
			OccurredAt: time.Date(2026, 8, 2, 15, 15, 44, 0, time.UTC),
		}},
		{name: "invalid provider error", providerErr: &streams.ProviderError{
			Source:         streams.ProviderErrorSourceOpenCodeStderr,
			RemediationURL: "https://opencode.ai/workspace/wrk_123/go",
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc.createRecoveryStatusMessage(ctx, watcher.AgentEventData{
				TaskID:        "t-none",
				SessionID:     "s-none",
				ErrorMessage:  "provider failed",
				ProviderError: tt.providerErr,
			})
			if len(mc.sessionMessages) == 0 {
				t.Fatal("expected a session message")
			}
			if url, ok := mc.sessionMessages[0].metadata["remediation_url"]; ok && url != "" {
				t.Fatalf("remediation_url = %#v, want absent", url)
			}
			mc.sessionMessages = nil
		})
	}
}
