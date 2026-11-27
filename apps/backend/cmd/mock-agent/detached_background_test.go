package main

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

func detachedCompletions(updater *capturingUpdater, sessionID acp.SessionId) int {
	updater.mu.Lock()
	defer updater.mu.Unlock()
	count := 0
	for _, note := range updater.notes {
		if note.SessionId != sessionID || note.Update.UsageUpdate == nil {
			continue
		}
		origin, _ := note.Update.UsageUpdate.Meta[claudeOriginMetaKey].(map[string]any)
		if origin["kind"] == claudeOriginTaskNotification {
			count++
		}
	}
	return count
}

func startDetachedPrompt(t *testing.T, agent *mockAgent, sessionID acp.SessionId) {
	t.Helper()
	response, err := agent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: sessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock("/detached-background 1m")},
	})
	if err != nil || response.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("foreground prompt outcome = %v, %v", response, err)
	}
}

func TestMockAgentCancelCompletesDetachedWorkAfterForegroundEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const sid = acp.SessionId("detached-cancel")
		updater := newCapturingUpdater()
		agent := &mockAgent{model: "mock-fast", conn: updater, commandsEmitted: make(map[acp.SessionId]bool)}
		startDetachedPrompt(t, agent, sid)
		synctest.Wait()
		if got := detachedCompletions(updater, sid); got != 0 {
			t.Fatalf("detached work completed before cancellation: %d", got)
		}
		if err := agent.Cancel(context.Background(), acp.CancelNotification{SessionId: sid}); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		cancelledCompletions := detachedCompletions(updater, sid)
		time.Sleep(time.Minute)
		synctest.Wait()
		if cancelledCompletions != 1 {
			t.Errorf("detached completions after cancellation = %d, want 1 before natural completion", cancelledCompletions)
		}
		if got := detachedCompletions(updater, sid); got != 1 {
			t.Fatalf("cancelled timer emitted another completion: %d", got)
		}
	})
}

func TestMockAgentDetachedCancellationIsSessionScoped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		updater := newCapturingUpdater()
		agent := &mockAgent{model: "mock-fast", conn: updater, commandsEmitted: make(map[acp.SessionId]bool)}
		startDetachedPrompt(t, agent, "first")
		startDetachedPrompt(t, agent, "second")
		if err := agent.Cancel(context.Background(), acp.CancelNotification{SessionId: "first"}); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		first, second := detachedCompletions(updater, "first"), detachedCompletions(updater, "second")
		if err := agent.Cancel(context.Background(), acp.CancelNotification{SessionId: "second"}); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		secondCancelled := detachedCompletions(updater, "second")
		time.Sleep(time.Minute)
		synctest.Wait()
		if first != 1 || second != 0 {
			t.Errorf("session completions = %d, %d, want 1, 0", first, second)
		}
		if secondCancelled != 1 {
			t.Fatalf("second session completions = %d, want 1", secondCancelled)
		}
	})
}

func TestMockAgentNaturalDetachedCompletionIsNotRepeatedByCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const sid = acp.SessionId("detached-natural")
		updater := newCapturingUpdater()
		agent := &mockAgent{model: "mock-fast", conn: updater, commandsEmitted: make(map[acp.SessionId]bool)}
		startDetachedPrompt(t, agent, sid)
		time.Sleep(time.Minute)
		synctest.Wait()
		if got := detachedCompletions(updater, sid); got != 1 {
			t.Fatalf("natural completions = %d, want 1", got)
		}
		if err := agent.Cancel(context.Background(), acp.CancelNotification{SessionId: sid}); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if got := detachedCompletions(updater, sid); got != 1 {
			t.Fatalf("completed workload emitted again on cancel: %d", got)
		}
	})
}
