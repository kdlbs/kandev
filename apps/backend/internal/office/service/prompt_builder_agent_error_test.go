package service_test

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/service"
)

// TestBuildPrompt_AgentError_NamesFailedAgentAndError is the regression for
// the CEO agent_error wake always rendering "Error: unknown" — the prompt
// builder read pc.RecentErrors, which production never populates. It must
// now name the failed agent/session and the real error text.
func TestBuildPrompt_AgentError_NamesFailedAgentAndError(t *testing.T) {
	pc := &service.PromptContext{
		Reason:            service.RunReasonAgentError,
		FailedAgentID:     "worker-1",
		FailedSessionID:   "sess-1",
		AgentErrorMessage: "exit status 1: context deadline exceeded",
	}
	prompt := service.BuildPrompt(pc)

	for _, want := range []string{"worker-1", "sess-1", "exit status 1: context deadline exceeded"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("agent_error prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "unknown") {
		t.Errorf("agent_error prompt should not fall back to the generic placeholder:\n%s", prompt)
	}
}

// TestBuildPrompt_AgentError_FallsBackToRecentErrors covers the fallback
// order: when AgentErrorMessage is unset, use RecentErrors[0] (used by
// callers that still populate the heartbeat-style field).
func TestBuildPrompt_AgentError_FallsBackToRecentErrors(t *testing.T) {
	pc := &service.PromptContext{
		Reason:       service.RunReasonAgentError,
		RecentErrors: []string{"session timeout"},
	}
	prompt := service.BuildPrompt(pc)
	if !strings.Contains(prompt, "session timeout") {
		t.Errorf("agent_error prompt should fall back to RecentErrors[0]:\n%s", prompt)
	}
}

// TestBuildPrompt_AgentError_FallsBackToUnknown covers the last-resort
// placeholder when no error info is available at all.
func TestBuildPrompt_AgentError_FallsBackToUnknown(t *testing.T) {
	pc := &service.PromptContext{Reason: service.RunReasonAgentError}
	prompt := service.BuildPrompt(pc)
	if !strings.Contains(prompt, "unknown") {
		t.Errorf("agent_error prompt should fall back to \"unknown\" with no error info:\n%s", prompt)
	}
}
