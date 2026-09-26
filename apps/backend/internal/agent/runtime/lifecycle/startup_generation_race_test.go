package lifecycle

import (
	"context"
	"testing"
	"time"
)

func TestStreamDisconnectBeforeReplacementCarriesStartupGeneration(t *testing.T) {
	execution := &AgentExecution{
		ID:           "exec-before-replacement",
		promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	startupGeneration := execution.beginStartupAttempt()
	streamManager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)

	streamManager.handleUpdatesDisconnectWithGeneration(
		execution,
		context.Canceled,
		startupGeneration,
	)

	select {
	case signal := <-execution.promptDoneCh:
		if signal.StartupGeneration != startupGeneration {
			t.Fatalf("startup generation = %d, want %d", signal.StartupGeneration, startupGeneration)
		}
	case <-time.After(time.Second):
		t.Fatal("disconnect did not signal the owning prompt")
	}
}

func TestDelayedStreamDisconnectCannotSignalReplacementPrompt(t *testing.T) {
	execution := &AgentExecution{
		ID:           "exec-after-replacement",
		promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	oldStartupGeneration := execution.beginStartupAttempt()
	newStartupGeneration, ok := execution.beginStartupRecovery()
	if !ok {
		t.Fatal("expected startup recovery generation")
	}

	streamManager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	streamManager.handleUpdatesDisconnectWithGeneration(
		execution,
		context.Canceled,
		oldStartupGeneration,
	)

	select {
	case signal := <-execution.promptDoneCh:
		t.Fatalf("stale disconnect signaled generation %d after replacement generation %d", signal.StartupGeneration, newStartupGeneration)
	default:
	}
}

func TestWaitForPromptDoneRejectsStaleStartupGenerationWithWildcardPrompt(t *testing.T) {
	execution := &AgentExecution{
		ID:           "exec-wildcard-prompt",
		promptDoneCh: make(chan PromptCompletionSignal, 2),
	}
	oldStartupGeneration := execution.beginStartupAttempt()
	newStartupGeneration, ok := execution.beginStartupRecovery()
	if !ok {
		t.Fatal("expected startup recovery generation")
	}

	execution.promptDoneCh <- PromptCompletionSignal{
		IsError:           true,
		Error:             "old stream disconnected",
		StartupGeneration: oldStartupGeneration,
	}
	execution.promptDoneCh <- PromptCompletionSignal{
		StopReason:        "end_turn",
		StartupGeneration: newStartupGeneration,
	}

	sessionManager := NewSessionManager(newSessionTestLogger(), make(chan struct{}))
	result, err := sessionManager.waitForPromptDone(context.Background(), execution, 0)
	if err != nil {
		t.Fatalf("waitForPromptDone: %v", err)
	}
	if result == nil || result.StopReason != "end_turn" {
		t.Fatalf("result = %+v, want replacement completion", result)
	}
}
