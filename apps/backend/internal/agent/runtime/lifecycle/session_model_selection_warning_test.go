package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/agent"
)

func TestInitializeAndPrompt_UniqueVariationPublishesProviderNeutralWarning(t *testing.T) {
	mock := newMockAgentServer(t)
	defer mock.Close()

	log := newSessionTestLogger()
	stopCh := newTestStopCh(t)
	sm := NewSessionManager(log, stopCh)
	streamMgr := NewStreamManager(log, StreamCallbacks{
		OnAgentEvent: func(execution *AgentExecution, event agentctl.AgentEvent) {},
	}, nil, stopCh)
	cleanupStreamManager(t, stopCh, streamMgr)
	eventBus := &MockEventBusWithTracking{}
	sm.SetDependencies(NewEventPublisher(eventBus, log), streamMgr, nil, nil)

	client := createTestClient(t, mock.server.URL)
	defer client.Close()
	execution := &AgentExecution{
		ID:            "exec-unique-variation",
		TaskID:        "task-1",
		SessionID:     "session-1",
		WorkspacePath: "/workspace",
		agentctl:      client,
		promptDoneCh:  make(chan PromptCompletionSignal, 1),
	}
	execution.SetModelState(modelState("opus[1m]"))
	agentConfig := &testAgent{
		id:      "test-agent",
		enabled: true,
		runtimeConfig: &agents.RuntimeConfig{
			Cmd:            agents.NewCommand("test-agent"),
			Protocol:       agent.ProtocolACP,
			SessionConfig:  agents.SessionConfig{},
			ResourceLimits: agents.ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := sm.InitializeAndPrompt(ctx, execution, agentConfig, "", nil, nil,
		func(executionID string) error { return nil },
		StartModelPolicy{Model: "opus"}, "plan", nil)
	if err != nil {
		t.Fatalf("InitializeAndPrompt failed: %v", err)
	}

	setModelCalls := 0
	for _, action := range mock.getActionLog() {
		if action == "agent.session.set_model" {
			setModelCalls++
		}
	}
	if setModelCalls != 1 {
		t.Fatalf("set_model calls = %d, want exactly 1", setModelCalls)
	}

	var warning *streams.ModelSelectionWarning
	legacyFallbackEvents := 0
	for _, event := range eventBus.getStreamEvents() {
		if event.Data == nil {
			continue
		}
		if event.Data.ModelSelectionWarning != nil {
			warning = event.Data.ModelSelectionWarning
		}
		if event.Data.Type == streams.EventTypeSessionModelFallback {
			legacyFallbackEvents++
		}
	}
	if warning == nil {
		t.Fatal("expected a model-selection warning event")
	}
	if warning.RequestedModel != "opus" {
		t.Errorf("warning requested model = %q, want opus", warning.RequestedModel)
	}
	if warning.EffectiveModel != "opus[1m]" {
		t.Errorf("warning effective model = %q, want opus[1m]", warning.EffectiveModel)
	}
	if warning.FallbackModel != "" {
		t.Errorf("warning fallback model = %q, want empty", warning.FallbackModel)
	}
	if warning.Reason != ModelSelectionReasonUniqueVariationApplied {
		t.Errorf("warning reason = %q, want %q", warning.Reason, ModelSelectionReasonUniqueVariationApplied)
	}
	if legacyFallbackEvents != 0 {
		t.Fatalf("legacy fallback events = %d, want zero for unique variation", legacyFallbackEvents)
	}
}
