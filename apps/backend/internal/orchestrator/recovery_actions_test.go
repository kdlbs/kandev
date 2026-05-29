package orchestrator

import (
	"context"
	"strings"
	"testing"

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
