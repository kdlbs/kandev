package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-AGENTS-AGENT-STALL-RECOVERY-001.12
func TestWaitForPromptDone_AcceptedSilentPromptIsAdvisory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		eventBus := &MockEventBusWithTracking{}
		sm := NewSessionManager(newSessionTestLogger(), make(chan struct{}))
		sm.eventPublisher = NewEventPublisher(eventBus, newSessionTestLogger())
		execution := &AgentExecution{
			ID: "execution", TaskID: "task", SessionID: "session",
			promptDoneCh: make(chan PromptCompletionSignal, 1),
			Status:       v1.AgentStatusRunning,
		}
		generation := beginExecutionPrompt(execution)
		sm.markPromptDispatched(execution, generation)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := sm.waitForPromptDone(ctx, execution, generation)
			done <- err
		}()
		time.Sleep(5 * time.Minute)
		synctest.Wait()

		payload := lastStalledPayload(t, eventBus)
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		if fields["prompt_accepted"] != true || fields["output_observed"] != false {
			t.Fatalf("stall must distinguish accepted silence from output: %s", encoded)
		}
		if evidence := execution.promptAttemptEvidenceSnapshot(); evidence.OutputObserved || evidence.EffectObserved {
			t.Fatal("acceptance created replay-safety evidence")
		}
		select {
		case err := <-done:
			t.Fatalf("inactivity settled the prompt: %v", err)
		default:
		}
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		if got := countPublishedEvents(eventBus, "agent.stalled"); got != 1 {
			t.Fatalf("stall notices = %d, want 1", got)
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("wait error = %v, want cancellation", err)
		}
	})
}
