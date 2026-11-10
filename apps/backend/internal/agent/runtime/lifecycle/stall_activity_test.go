package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestWaitForPromptDone_MetadataOnlyStreamStillReportsNeverStarted covers
// @covers AC-AGENTS-AGENT-STALL-RECOVERY-001.9: a metadata-only stream (no
// turn-content event) must not keep resetting the stall clock, so the
// watchdog still fires five minutes after dispatch with NeverStarted true.
func TestWaitForPromptDone_MetadataOnlyStreamStillReportsNeverStarted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		eventBus := &MockEventBusWithTracking{}
		sm := NewSessionManager(newSessionTestLogger(), make(chan struct{}))
		sm.eventPublisher = NewEventPublisher(eventBus, newSessionTestLogger())
		execution := &AgentExecution{
			promptGeneration: 7,
			ID:               "test-exec",
			TaskID:           "test-task",
			SessionID:        "test-session",
			promptDoneCh:     make(chan PromptCompletionSignal, 1),
			Status:           v1.AgentStatusRunning,
		}
		execution.lastActivityAt = time.Now()

		mgr := &Manager{eventPublisher: sm.eventPublisher}

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		waitResult := make(chan error, 1)
		go func() {
			_, err := sm.waitForPromptDone(ctx, execution, 7)
			waitResult <- err
		}()

		stop := make(chan struct{})
		defer close(stop)
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					mgr.recordActivity(execution, agentctl.AgentEvent{Type: "usage_update"})
				case <-stop:
					return
				}
			}
		}()

		time.Sleep(5 * time.Minute)
		synctest.Wait()

		payload := lastStalledPayload(t, eventBus)
		if !payload.NeverStarted {
			t.Fatalf("NeverStarted = false, want true: a metadata-only stream must not reset the clock")
		}
		if got := countPublishedEvents(eventBus, "agent.stalled"); got != 1 {
			t.Fatalf("published stalled events = %d, want 1", got)
		}

		cancel()
		if err := <-waitResult; !errors.Is(err, context.Canceled) {
			t.Fatalf("waitForPromptDone error = %v, want context canceled", err)
		}
	})
}

// TestRecordActivity_MetadataFrameDoesNotAdvancePromptProgress covers
// @covers AC-AGENTS-AGENT-STALL-RECOVERY-001.2: recordActivity must only move
// lastActivityAt for a turnContentEventTypes event; a metadata frame leaves
// the snapshot's timestamp and agentEventSincePrompt untouched.
func TestRecordActivity_MetadataFrameDoesNotAdvancePromptProgress(t *testing.T) {
	execution := &AgentExecution{
		ID:     "test-exec",
		Status: v1.AgentStatusRunning,
	}
	dispatchTime := time.Now().Add(-time.Minute)
	execution.lastActivityAt = dispatchTime

	mgr := &Manager{eventPublisher: NewEventPublisher(&MockEventBusWithTracking{}, newSessionTestLogger())}

	mgr.recordActivity(execution, agentctl.AgentEvent{Type: "usage_update"})

	lastActivity, agentEventSeen, _ := execution.promptActivitySnapshot()
	if !lastActivity.Equal(dispatchTime) {
		t.Fatalf("lastActivityAt = %v, want unchanged dispatch time %v after a metadata frame", lastActivity, dispatchTime)
	}
	if agentEventSeen {
		t.Fatal("agentEventSincePrompt = true after a metadata-only frame")
	}

	mgr.recordActivity(execution, agentctl.AgentEvent{Type: "message_chunk"})

	lastActivity, agentEventSeen, _ = execution.promptActivitySnapshot()
	if !lastActivity.After(dispatchTime) {
		t.Fatalf("lastActivityAt did not advance after a turn-content event")
	}
	if !agentEventSeen {
		t.Fatal("agentEventSincePrompt = false after a turn-content event")
	}
}

// @covers AC-AGENTS-AGENT-STALL-RECOVERY-001.13
func TestPromptAcceptancePreservesEarlyOutputEvidence(t *testing.T) {
	for _, withStore := range []bool{false, true} {
		t.Run(fmt.Sprintf("store=%t", withStore), func(t *testing.T) {
			sm := NewSessionManager(newSessionTestLogger(), make(chan struct{}))
			execution := &AgentExecution{ID: "early-output", SessionID: "session"}
			generation := beginExecutionPrompt(execution)
			if withStore {
				sm.executionStore = NewExecutionStore()
				if err := sm.executionStore.Add(execution); err != nil {
					t.Fatal(err)
				}
			}
			execution.markAgentActivity()
			beforeTime, _, beforeEpoch := execution.promptActivitySnapshot()

			sm.markPromptDispatched(execution, generation)

			evidence := execution.promptAttemptEvidenceSnapshot()
			if !evidence.OutputObserved || !evidence.EffectObserved {
				t.Fatal("acceptance erased output/effect evidence from the same turn")
			}
			afterTime, _, afterEpoch := execution.promptActivitySnapshot()
			if !afterTime.Equal(beforeTime) || afterEpoch != beforeEpoch {
				t.Fatal("acceptance changed the progress clock after real output")
			}
		})
	}
}

// @covers AC-AGENTS-AGENT-STALL-RECOVERY-001.13
func TestBeginPromptClearsPreviousEvidence(t *testing.T) {
	execution := &AgentExecution{ID: "new-turn", SessionID: "session"}
	store := NewExecutionStore()
	if err := store.Add(execution); err != nil {
		t.Fatal(err)
	}
	execution.markAgentActivity()
	generation, err := store.BeginPrompt(execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	evidence := execution.promptAttemptEvidenceSnapshot()
	if evidence.OutputObserved || evidence.EffectObserved {
		t.Fatal("a new prompt inherited its predecessor's output/effect evidence")
	}
	_, _, beforeEpoch := execution.promptActivitySnapshot()
	store.MarkPromptDispatched(execution.ID, generation)
	at, seen, epoch := execution.promptActivitySnapshot()
	if at.IsZero() || seen || epoch != beforeEpoch+1 {
		t.Fatal("first acceptance must arm the clock without inventing output")
	}
}
