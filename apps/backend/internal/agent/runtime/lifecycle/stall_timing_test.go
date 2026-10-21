package lifecycle

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestStallTimingOverrideRequiresE2EProfile(t *testing.T) {
	for _, profile := range []string{"", "true"} {
		t.Run("e2e="+profile, func(t *testing.T) {
			t.Setenv("KANDEV_E2E_MOCK", profile)
			t.Setenv("KANDEV_E2E_PROMPT_STALL_TIMEOUT", "1s")
			synctest.Test(t, func(t *testing.T) {
				bus := &MockEventBusWithTracking{}
				sm := NewSessionManager(newSessionTestLogger(), make(chan struct{}))
				sm.eventPublisher = NewEventPublisher(bus, newSessionTestLogger())
				execution := &AgentExecution{ID: "execution", promptDoneCh: make(chan PromptCompletionSignal, 1)}
				generation := beginExecutionPrompt(execution)
				sm.markPromptDispatched(execution, generation)
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan struct{})
				go func() {
					defer close(done)
					_, _ = sm.waitForPromptDone(ctx, execution, generation)
				}()
				defer func() { cancel(); <-done }()
				time.Sleep(2 * time.Second)
				synctest.Wait()
				want := 0
				if profile == "true" {
					want = 1
				}
				if got := countPublishedEvents(bus, "agent.stalled"); got != want {
					t.Fatalf("stall events = %d, want %d", got, want)
				}
			})
		})
	}
}
