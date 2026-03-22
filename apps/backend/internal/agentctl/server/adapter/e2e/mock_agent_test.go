//go:build e2e

package e2e

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/pkg/agent"
)

// TestMockAgent_BasicPrompt validates the harness works end-to-end using
// the mock agent. No API cost — can always run.
func TestMockAgent_BasicPrompt(t *testing.T) {
	binary := buildMockAgent(t)

	result := RunAgent(t, AgentSpec{
		Name:          "mock-agent",
		Command:       binary + " --model mock-fast",
		Protocol:      agent.ProtocolACP,
		DefaultPrompt: "hello",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertSessionIDConsistent(t, result.Events)

	t.Logf("mock agent completed in %s with %d events", result.Duration, len(result.Events))
}

// TestMockAgent_SimpleMessage tests the deterministic /e2e:simple-message scenario.
// Validates: thinking event + text message event + complete event.
func TestMockAgent_SimpleMessage(t *testing.T) {
	binary := buildMockAgent(t)

	result := RunAgent(t, AgentSpec{
		Name:          "mock-agent",
		Command:       binary + " --model mock-fast",
		Protocol:      agent.ProtocolACP,
		DefaultPrompt: "/e2e:simple-message",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertNoErrors(t, result.Events)
	AssertHasEventType(t, result.Events, adapter.EventTypeReasoning)
	AssertHasEventType(t, result.Events, adapter.EventTypeMessageChunk)

	// Verify specific content
	hasThinkingContent := false
	hasTextContent := false
	for _, ev := range result.Events {
		if ev.Type == adapter.EventTypeReasoning && ev.ReasoningText == "Processing the request..." {
			hasThinkingContent = true
		}
		if ev.Type == adapter.EventTypeMessageChunk && ev.Text == "This is a simple mock response for e2e testing." {
			hasTextContent = true
		}
	}
	if !hasThinkingContent {
		t.Error("expected thinking event with 'Processing the request...'")
	}
	if !hasTextContent {
		t.Error("expected text event with 'This is a simple mock response for e2e testing.'")
	}
}

// TestMockAgent_ToolCallEvents tests tool call start + complete flow via /e2e:read-and-edit.
// Validates: tool_call and tool_update events are emitted with correct types.
func TestMockAgent_ToolCallEvents(t *testing.T) {
	binary := buildMockAgent(t)

	result := RunAgent(t, AgentSpec{
		Name:          "mock-agent",
		Command:       binary + " --model mock-fast",
		Protocol:      agent.ProtocolACP,
		DefaultPrompt: "/e2e:read-and-edit",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertNoErrors(t, result.Events)
	AssertHasEventType(t, result.Events, adapter.EventTypeToolCall)
	AssertHasEventType(t, result.Events, adapter.EventTypeToolUpdate)

	// Count tool calls and updates
	counts := CountEventsByType(result.Events)
	if counts[adapter.EventTypeToolCall] < 2 {
		t.Errorf("expected at least 2 tool_call events (read + edit), got %d", counts[adapter.EventTypeToolCall])
	}

	t.Logf("event counts: %v", counts)
}

// TestMockAgent_PermissionFlow tests auto-approved permission requests via /e2e:permission-flow.
func TestMockAgent_PermissionFlow(t *testing.T) {
	binary := buildMockAgent(t)

	result := RunAgent(t, AgentSpec{
		Name:          "mock-agent",
		Command:       binary + " --model mock-fast",
		Protocol:      agent.ProtocolACP,
		DefaultPrompt: "/e2e:permission-flow",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertNoErrors(t, result.Events)

	// With auto-approve, the tool call should complete successfully
	AssertHasEventType(t, result.Events, adapter.EventTypeToolCall)

	// Look for the "Permission was granted" text
	hasGrantedText := false
	for _, ev := range result.Events {
		if ev.Type == adapter.EventTypeMessageChunk && ev.Text == "Permission was granted and command executed." {
			hasGrantedText = true
		}
	}
	if !hasGrantedText {
		t.Error("expected 'Permission was granted and command executed.' message (auto-approve should grant)")
	}
}

// TestMockAgent_AllTools tests one of every tool kind via /e2e:all-tools.
// Validates: read, search, edit, execute, fetch tool kinds all appear.
func TestMockAgent_AllTools(t *testing.T) {
	binary := buildMockAgent(t)

	result := RunAgent(t, AgentSpec{
		Name:          "mock-agent",
		Command:       binary + " --model mock-fast",
		Protocol:      agent.ProtocolACP,
		DefaultPrompt: "/e2e:all-tools",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertNoErrors(t, result.Events)

	// Check that multiple tool kinds are present
	toolNames := map[string]bool{}
	for _, ev := range result.Events {
		if ev.Type == adapter.EventTypeToolCall && ev.ToolName != "" {
			toolNames[ev.ToolName] = true
		}
	}

	// ACP tool kinds map to ToolName in events
	expectedKinds := []string{"read", "search", "edit", "execute", "fetch"}
	for _, kind := range expectedKinds {
		if !toolNames[kind] {
			t.Errorf("expected tool kind %q in events, got tools: %v", kind, toolNames)
		}
	}
}

// TestMockAgent_MultiTurn sends two prompts to the same session.
// Validates: the agent handles multiple prompts without errors and session ID stays consistent.
func TestMockAgent_MultiTurn(t *testing.T) {
	binary := buildMockAgent(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	setup := setupAgentProcess(t, ctx, AgentSpec{
		Name:     "mock-agent",
		Command:  binary + " --model mock-fast",
		Protocol: agent.ProtocolACP,
	})

	// First turn
	events1 := collectPromptEvents(ctx, t, setup, "/e2e:simple-message")
	assertEventsHaveType(t, events1, adapter.EventTypeComplete, "turn 1")
	assertEventsNoErrors(t, events1, "turn 1")

	sessionID := setup.adpt.GetSessionID()
	if sessionID == "" {
		t.Fatal("session ID is empty after first turn")
	}

	// Second turn — same session
	events2 := collectPromptEvents(ctx, t, setup, "/e2e:multi-turn")
	assertEventsHaveType(t, events2, adapter.EventTypeComplete, "turn 2")
	assertEventsNoErrors(t, events2, "turn 2")

	// Session ID must remain the same
	if setup.adpt.GetSessionID() != sessionID {
		t.Errorf("session ID changed between turns: %s -> %s", sessionID, setup.adpt.GetSessionID())
	}

	t.Logf("multi-turn: turn 1 had %d events, turn 2 had %d events", len(events1), len(events2))
}

// TestMockAgent_SessionReset tests creating a new session on the same connection.
// Validates: ResetSession works and produces a different session ID.
func TestMockAgent_SessionReset(t *testing.T) {
	binary := buildMockAgent(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	setup := setupAgentProcess(t, ctx, AgentSpec{
		Name:     "mock-agent",
		Command:  binary + " --model mock-fast",
		Protocol: agent.ProtocolACP,
	})

	// First turn
	events1 := collectPromptEvents(ctx, t, setup, "/e2e:simple-message")
	assertEventsHaveType(t, events1, adapter.EventTypeComplete, "turn 1")

	firstSessionID := setup.adpt.GetSessionID()

	// Reset session (creates new session on same connection)
	resettable, ok := setup.adpt.(adapter.SessionResettableAdapter)
	if !ok {
		t.Fatal("adapter does not implement SessionResettableAdapter")
	}

	newSessionID, err := resettable.ResetSession(ctx, nil)
	if err != nil {
		t.Fatalf("ResetSession failed: %v", err)
	}

	if newSessionID == "" {
		t.Fatal("new session ID is empty after reset")
	}

	// Drain events from ResetSession (session_status etc.)
	drainEvents(ctx, setup, 200*time.Millisecond)

	// Prompt on the new session
	events2 := collectPromptEvents(ctx, t, setup, "/e2e:simple-message")
	assertEventsHaveType(t, events2, adapter.EventTypeComplete, "post-reset turn")
	assertEventsNoErrors(t, events2, "post-reset turn")

	t.Logf("session reset: %s -> %s", firstSessionID, newSessionID)
}

// TestMockAgent_LoadSession tests resuming a session via LoadSession.
// Validates: LoadSession succeeds and subsequent prompts work.
func TestMockAgent_LoadSession(t *testing.T) {
	binary := buildMockAgent(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	setup := setupAgentProcess(t, ctx, AgentSpec{
		Name:     "mock-agent",
		Command:  binary + " --model mock-fast",
		Protocol: agent.ProtocolACP,
	})

	// First turn to establish a session
	events1 := collectPromptEvents(ctx, t, setup, "/e2e:simple-message")
	assertEventsHaveType(t, events1, adapter.EventTypeComplete, "initial turn")

	sessionID := setup.adpt.GetSessionID()
	if sessionID == "" {
		t.Fatal("session ID is empty")
	}

	// Load the same session (simulates resume after process restart)
	if err := setup.adpt.LoadSession(ctx, sessionID, nil); err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}

	// Drain events from LoadSession (session_status etc.)
	drainEvents(ctx, setup, 200*time.Millisecond)

	// Prompt on the loaded session
	events2 := collectPromptEvents(ctx, t, setup, "/e2e:simple-message")
	assertEventsHaveType(t, events2, adapter.EventTypeComplete, "post-load turn")
	assertEventsNoErrors(t, events2, "post-load turn")

	// Session ID should match
	if setup.adpt.GetSessionID() != sessionID {
		t.Errorf("session ID changed after LoadSession: %s -> %s", sessionID, setup.adpt.GetSessionID())
	}

	t.Logf("session load: resumed %s successfully", sessionID)
}

// TestMockAgent_ThinkingEvents tests extended thinking/reasoning via /thinking.
func TestMockAgent_ThinkingEvents(t *testing.T) {
	binary := buildMockAgent(t)

	result := RunAgent(t, AgentSpec{
		Name:          "mock-agent",
		Command:       binary + " --model mock-fast",
		Protocol:      agent.ProtocolACP,
		DefaultPrompt: "/thinking",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertNoErrors(t, result.Events)

	// Should have multiple reasoning events (the scenario emits 5 thoughts)
	reasoningCount := 0
	for _, ev := range result.Events {
		if ev.Type == adapter.EventTypeReasoning {
			reasoningCount++
		}
	}
	if reasoningCount < 3 {
		t.Errorf("expected at least 3 reasoning events, got %d", reasoningCount)
	}

	t.Logf("thinking: %d reasoning events emitted", reasoningCount)
}

// --- Multi-turn helpers ---

// agentSetup holds the state for multi-turn tests.
type agentSetup struct {
	mgr  managerInterface
	adpt adapter.AgentAdapter
}

// managerInterface is the subset of process.Manager needed by helpers.
type managerInterface interface {
	GetUpdates() <-chan adapter.AgentEvent
}

// setupAgentProcess initializes an agent process for multi-turn testing.
// Returns the setup with manager and adapter ready for prompting.
func setupAgentProcess(t *testing.T, ctx context.Context, spec AgentSpec) *agentSetup {
	t.Helper()
	requireBinary(t, spec.Command)

	workDir := setupWorkspace(t)
	cfg := buildInstanceConfig(spec.Command, spec.Protocol, workDir, spec.AutoApprove, spec.ContinueCommand)
	log := newTestLogger(t)

	mgr := process.NewManager(cfg, log)
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("failed to start agent: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = mgr.Stop(stopCtx)
	})

	adpt := mgr.GetAdapter()
	if adpt == nil {
		t.Fatal("adapter is nil after Start")
	}
	if err := adpt.Initialize(ctx); err != nil {
		t.Fatalf("failed to initialize adapter: %v", err)
	}
	if _, err := adpt.NewSession(ctx, nil); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	return &agentSetup{mgr: mgr, adpt: adpt}
}

// collectPromptEvents sends a prompt and collects all events until the turn completes.
// It uses a dedicated context to stop the collector goroutine cleanly between calls.
func collectPromptEvents(ctx context.Context, t *testing.T, setup *agentSetup, prompt string) []adapter.AgentEvent {
	t.Helper()

	var events []adapter.AgentEvent
	var mu sync.Mutex
	collectCtx, collectCancel := context.WithCancel(ctx)

	go func() {
		ch := setup.mgr.GetUpdates()
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return
				}
				mu.Lock()
				events = append(events, ev)
				mu.Unlock()
			case <-collectCtx.Done():
				return
			}
		}
	}()

	if err := setup.adpt.Prompt(ctx, prompt, nil); err != nil {
		collectCancel()
		t.Fatalf("prompt failed: %v", err)
	}

	// Grace period for remaining events to propagate
	time.Sleep(500 * time.Millisecond)
	collectCancel() // Stop the collector goroutine

	mu.Lock()
	defer mu.Unlock()
	result := make([]adapter.AgentEvent, len(events))
	copy(result, events)
	return result
}

// drainEvents reads and discards events from the updates channel for the given duration.
// Use between operations that emit events (e.g., ResetSession, LoadSession) to avoid
// stale events being picked up by the next collectPromptEvents call.
func drainEvents(ctx context.Context, setup *agentSetup, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	ch := setup.mgr.GetUpdates()
	for {
		select {
		case <-ch:
			// discard
		case <-timer.C:
			return
		case <-ctx.Done():
			return
		}
	}
}

// assertEventsHaveType checks that at least one event of the given type exists.
func assertEventsHaveType(t *testing.T, events []adapter.AgentEvent, eventType, context string) {
	t.Helper()
	for _, ev := range events {
		if ev.Type == eventType {
			return
		}
	}
	t.Errorf("[%s] expected at least one %q event, got none (total: %d events)", context, eventType, len(events))
}

// assertEventsNoErrors checks that no error events were received.
func assertEventsNoErrors(t *testing.T, events []adapter.AgentEvent, context string) {
	t.Helper()
	for _, ev := range events {
		if ev.Type == adapter.EventTypeError {
			t.Errorf("[%s] unexpected error event: %s", context, ev.Error)
		}
	}
}
