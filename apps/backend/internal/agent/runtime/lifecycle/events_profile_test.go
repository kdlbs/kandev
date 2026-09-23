package lifecycle

import (
	"encoding/json"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

func TestAgentEventPayloadSeparatesOfficeAndExecutionProfiles(t *testing.T) {
	payload := newAgentEventPayload(&AgentExecution{
		ID: "exec-1", AgentProfileID: "claude-opus", OfficeAgentProfileID: "office-cto",
	})
	if payload.AgentProfileID != "office-cto" {
		t.Fatalf("agent profile = %q, want stable Office identity", payload.AgentProfileID)
	}
	if payload.ExecutionProfileID != "claude-opus" {
		t.Fatalf("execution profile = %q, want concrete CLI profile", payload.ExecutionProfileID)
	}
}

func TestAgentEventPayloadCarriesProviderErrorAndAgentID(t *testing.T) {
	occurred := time.Date(2026, 8, 2, 15, 15, 44, 0, time.UTC)
	payload := newAgentEventPayload(&AgentExecution{
		ID:      "exec-1",
		AgentID: "opencode-acp",
		ProviderError: &streams.ProviderError{
			Source:     streams.ProviderErrorSourceOpenCodeStderr,
			ModelID:    "kimi-k3",
			Message:    "5-hour usage limit reached",
			OccurredAt: occurred,
		},
	})
	if payload.AgentID != "opencode-acp" {
		t.Fatalf("agent ID = %q, want opencode-acp", payload.AgentID)
	}
	if payload.ProviderError == nil || payload.ProviderError.ModelID != "kimi-k3" {
		t.Fatalf("provider error = %+v", payload.ProviderError)
	}
}

func TestAgentEventPayloadCarriesRunID(t *testing.T) {
	payload := newAgentEventPayload(&AgentExecution{
		ID: "exec-1", WorkspaceID: "ws-1", RunID: "run-1", RunSessionID: "run-session-1", RunAttempt: 2,
	})
	if payload.RunID != "run-1" {
		t.Fatalf("run ID = %q, want run-1", payload.RunID)
	}
	if payload.OwnerKind != ExecutionOwnerRun || payload.WorkspaceID != "ws-1" ||
		payload.RunSessionID != "run-session-1" || payload.RunAttempt != 2 {
		t.Fatalf("run owner = %#v, want exact run identity", payload)
	}
}

func TestAgentEventPayloadCarriesPromptTurnID(t *testing.T) {
	execution := &AgentExecution{ID: "exec-1", promptTurnID: "turn-1"}
	payload := newAgentEventPayload(execution)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	var fields map[string]string
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if fields["turn_id"] != "turn-1" {
		t.Fatalf("turn_id = %q, want turn-1", fields["turn_id"])
	}
}

func TestAgentStreamEventCarriesFrozenExactProfileAttempt(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("execution-attempt", "task-original", "session-original")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	binding := &models.ExactProfileLaunchAttemptBinding{
		TaskID: "task-original", SessionID: "session-original", ExecutionID: execution.ID,
		AttemptID: "attempt-original", SessionIncarnationID: "incarnation-original",
		AgentProfileID: "profile-original", ProfileRevision: time.Unix(1_726_500_000, 0).UTC(), Generation: 7,
	}
	if err := mgr.AdmitExactProfileLaunchAttempt(execution.ID, binding, func(*models.ExactProfileLaunchAttemptBinding) (bool, error) {
		return true, nil
	}); err != nil {
		t.Fatalf("attach exact-profile attempt: %v", err)
	}
	binding.TaskID = "task-mutated"
	binding.SessionID = "session-mutated"
	binding.ExecutionID = "execution-mutated"
	binding.AttemptID = "attempt-mutated"
	binding.SessionIncarnationID = "incarnation-mutated"
	binding.AgentProfileID = "profile-mutated"
	binding.ProfileRevision = time.Time{}
	binding.Generation = 99

	mgr.eventPublisher.PublishAgentStreamEvent(execution, agentctl.AgentEvent{Type: "message_chunk", Text: "first output"})
	streamEvents := eventBus.getStreamEvents()
	if len(streamEvents) == 0 {
		t.Fatal("expected a stream event")
	}
	got := streamEvents[0].ExactProfileAttempt
	if got == nil || got.TaskID != "task-original" || got.SessionID != "session-original" ||
		got.ExecutionID != "execution-attempt" || got.AttemptID != "attempt-original" ||
		got.SessionIncarnationID != "incarnation-original" || got.AgentProfileID != "profile-original" ||
		got.ProfileRevision != time.Unix(1_726_500_000, 0).UTC() || got.Generation != 7 {
		t.Fatalf("stream attempt = %#v, want frozen admitted tuple", got)
	}
	got.TaskID = "payload-mutated"
	mgr.eventPublisher.PublishAgentStreamEvent(execution, agentctl.AgentEvent{Type: "message_chunk", Text: "second output"})
	streamEvents = eventBus.getStreamEvents()
	if len(streamEvents) != 2 || streamEvents[1].ExactProfileAttempt == nil || streamEvents[1].ExactProfileAttempt.TaskID != "task-original" {
		t.Fatalf("second stream attempt = %#v, want independent frozen copy", streamEvents)
	}
}

func TestAgentStreamEventOmitsExactProfileAttemptWithoutAdmission(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("execution-ordinary", "task-ordinary", "session-ordinary")
	mgr.eventPublisher.PublishAgentStreamEvent(execution, agentctl.AgentEvent{Type: "message_chunk", Text: "ordinary output"})
	streamEvents := eventBus.getStreamEvents()
	if len(streamEvents) != 1 || streamEvents[0].ExactProfileAttempt != nil {
		t.Fatalf("ordinary stream event = %#v, want no exact-profile attempt", streamEvents)
	}
}
