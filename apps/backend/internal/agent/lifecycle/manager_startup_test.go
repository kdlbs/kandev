package lifecycle

import (
	"context"
	"strings"
	"testing"
)

// mockAgentProfileResolver returns a profile pointing to the mock-agent.
type mockAgentProfileResolver struct {
	cliPassthrough bool
}

func (m *mockAgentProfileResolver) ResolveProfile(_ context.Context, profileID string) (*AgentProfileInfo, error) {
	return &AgentProfileInfo{
		ProfileID:      profileID,
		ProfileName:    "Test Profile",
		AgentID:        "mock-agent",
		AgentName:      "mock-agent",
		Model:          "mock-fast",
		CLIPassthrough: m.cliPassthrough,
	}, nil
}

func TestStartAgentProcess_NotFound(t *testing.T) {
	mgr := newTestManager()
	err := mgr.StartAgentProcess(context.Background(), "non-existent")
	if err == nil {
		t.Fatal("expected error for non-existent execution")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestStartAgentProcess_NonPassthrough_NoAgentctl(t *testing.T) {
	mgr := newTestManager()
	// Use a resolver that returns CLIPassthrough=false
	mgr.profileResolver = &mockAgentProfileResolver{cliPassthrough: false}

	execution := &AgentExecution{
		ID:             "exec-1",
		SessionID:      "sess-1",
		AgentProfileID: "profile-1",
	}
	mgr.executionStore.Add(execution)

	err := mgr.StartAgentProcess(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected error for missing agentctl")
	}
	if !strings.Contains(err.Error(), "no agentctl client") {
		t.Errorf("expected 'no agentctl client' error, got: %v", err)
	}
}

func TestStartAgentProcess_Passthrough_NotResumed(t *testing.T) {
	mgr := newTestManager()
	mgr.profileResolver = &mockAgentProfileResolver{cliPassthrough: true}

	execution := &AgentExecution{
		ID:               "exec-1",
		SessionID:        "sess-1",
		AgentProfileID:   "profile-1",
		isResumedSession: false,
	}
	mgr.executionStore.Add(execution)

	err := mgr.StartAgentProcess(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected error (no interactive runner)")
	}
	// startPassthroughSession path errors with "interactive runner not available for passthrough mode"
	if !strings.Contains(err.Error(), "interactive runner not available") {
		t.Errorf("expected 'interactive runner not available' error from startPassthroughSession, got: %v", err)
	}
}

func TestStartAgentProcess_Passthrough_Resumed(t *testing.T) {
	mgr := newTestManager()
	mgr.profileResolver = &mockAgentProfileResolver{cliPassthrough: true}

	execution := &AgentExecution{
		ID:               "exec-1",
		SessionID:        "sess-1",
		AgentProfileID:   "profile-1",
		isResumedSession: true,
	}
	mgr.executionStore.Add(execution)

	err := mgr.StartAgentProcess(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected error (no interactive runner)")
	}
	// ResumePassthroughSession returns "interactive runner not available" (without
	// "for passthrough mode"), while startPassthroughSession includes the suffix.
	// Assert we hit the resume path specifically.
	if !strings.Contains(err.Error(), "interactive runner not available") {
		t.Errorf("expected 'interactive runner not available' error, got: %v", err)
	}
	if strings.Contains(err.Error(), "for passthrough mode") {
		t.Errorf("expected ResumePassthroughSession path (no 'for passthrough mode' suffix), got: %v", err)
	}
}
