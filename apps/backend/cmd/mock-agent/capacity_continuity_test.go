package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

func TestRetainedCapacityReleaseBarrier(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "capacity.gate")
	t.Setenv("E2E_MOCK_AGENT_CAPACITY_GATE_FILE", gate)
	if err := os.WriteFile(gate, nil, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := waitRetainedCapacityRelease(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("closed barrier must wait and honor cancellation, got %v", err)
	}
	if err := os.Remove(gate); err != nil {
		t.Fatal(err)
	}
	if err := waitRetainedCapacityRelease(context.Background()); err != nil {
		t.Fatalf("released barrier must admit the provider response: %v", err)
	}
	t.Setenv("E2E_MOCK_AGENT_CAPACITY_GATE_FILE", "")
	if err := waitRetainedCapacityRelease(context.Background()); err != nil {
		t.Fatalf("ordinary mock scenarios must not need a barrier: %v", err)
	}
}

func TestParseRetainedCapacityCmd(t *testing.T) {
	for _, test := range []struct {
		prompt string
		name   string
		fails  int
		ok     bool
	}{
		{prompt: "/capacity-after-tools", name: "after-tools", fails: 1, ok: true},
		{prompt: "/capacity-completed-tools", name: "completed-tools", fails: 1, ok: true},
		{prompt: "/capacity-retry:2", name: "retry", fails: 2, ok: true},
		{prompt: "/capacity-cancel", name: "cancel", fails: 9, ok: true},
		{prompt: "/capacity-exhaust", name: "exhaust", fails: 9, ok: true},
		{prompt: "<kandev-system>context</kandev-system>/capacity-retry", name: "retry", fails: 1, ok: true},
		{prompt: "/capacity-unknown", ok: false},
	} {
		t.Run(test.prompt, func(t *testing.T) {
			got, ok := parseRetainedCapacityCmd(test.prompt)
			if ok != test.ok || (ok && (got.name != test.name || got.failTimes != test.fails)) {
				t.Fatalf("parseRetainedCapacityCmd(%q) = (%+v, %v)", test.prompt, got, ok)
			}
		})
	}
}

func TestHandleRetainedCapacityUsesAttestedRequestError(t *testing.T) {
	const sid acp.SessionId = "retained-capacity-error-test"
	_ = os.Remove(retainedCapacityCounterPath(sid, "retry"))
	t.Cleanup(func() { _ = os.Remove(retainedCapacityCounterPath(sid, "retry")) })
	a := &mockAgent{conn: &mockUpdater{}}
	_, err, handled := a.handleRetainedCapacity(context.Background(), sid, "/capacity-retry")
	if !handled {
		t.Fatal("handled = false, want capacity scenario")
	}
	requestErr, ok := err.(*acp.RequestError)
	if !ok || requestErr.Code != -32603 || requestErr.Message != retainedCapacityMessage {
		t.Fatalf("error = %#v, want retained capacity RequestError", err)
	}
	data, ok := requestErr.Data.(map[string]any)
	if !ok {
		t.Fatalf("request error data = %#v, want marker", requestErr.Data)
	}
	meta, ok := data["kandevMock"].(map[string]any)
	if !ok || meta["retainedProviderCapacity"] != true {
		t.Fatalf("request error marker = %#v, want retainedProviderCapacity", data)
	}
}

func TestRetainedCapacityCountersAreIndependentByScenario(t *testing.T) {
	const sid acp.SessionId = "retained-capacity-counter-isolation-test"
	clearRetainedCapacityCounters(sid)
	t.Cleanup(func() { clearRetainedCapacityCounters(sid) })
	a := &mockAgent{conn: &mockUpdater{}}
	_, afterToolsErr, afterToolsHandled := a.handleRetainedCapacity(context.Background(), sid, "/capacity-after-tools")
	if !afterToolsHandled || afterToolsErr == nil {
		t.Fatalf("after-tools handled=%v error=%v, want one failure", afterToolsHandled, afterToolsErr)
	}
	_, retryErr, retryHandled := a.handleRetainedCapacity(context.Background(), sid, "/capacity-retry")
	if !retryHandled {
		t.Fatal("retry scenario was not handled")
	}
	requestErr, ok := retryErr.(*acp.RequestError)
	if !ok || requestErr.Code != -32603 {
		t.Fatalf("retry error = %#v, want its first marked request failure", retryErr)
	}
}

func TestRepeatedRetainedCapacityRetryCommandsReachSuccessAndResetNewEpisodes(t *testing.T) {
	const sid acp.SessionId = "retained-capacity-repeated-direct-retry-test"
	clearRetainedCapacityCounters(sid)
	t.Cleanup(func() { clearRetainedCapacityCounters(sid) })
	a := &mockAgent{conn: &mockUpdater{}}

	for attempt := 1; attempt <= 2; attempt++ {
		_, err, handled := a.handleRetainedCapacity(context.Background(), sid, "/capacity-retry:2")
		if !handled || err == nil {
			t.Fatalf("direct retry %d handled=%v error=%v, want a capacity failure", attempt, handled, err)
		}
	}
	if _, err, handled := a.handleRetainedCapacity(context.Background(), sid, "/capacity-retry:2"); !handled || err != nil {
		t.Fatalf("third direct retry handled=%v error=%v, want configured success", handled, err)
	}

	if _, err, handled := a.handleRetainedCapacity(context.Background(), sid, "/capacity-retry:2"); !handled || err == nil {
		t.Fatalf("new retry episode handled=%v error=%v, want a fresh first failure", handled, err)
	}
}

func TestHandleRetainedCapacityAfterToolsEmitsVisibleReadBeforeFailure(t *testing.T) {
	const sid acp.SessionId = "retained-capacity-tools-test"
	clearRetainedCapacityCounters(sid)
	t.Cleanup(func() { clearRetainedCapacityCounters(sid) })
	updater := &mockUpdater{}
	a := &mockAgent{conn: updater}
	_, err, handled := a.handleRetainedCapacity(context.Background(), sid, "/capacity-after-tools")
	if !handled || err == nil {
		t.Fatalf("handled=%v error=%v, want marked provider failure", handled, err)
	}
	updates := updater.getUpdates()
	if len(updates) != 4 || updates[0].notification.Update.AgentMessageChunk == nil ||
		updates[1].notification.Update.ToolCall == nil || updates[2].notification.Update.ToolCallUpdate == nil {
		t.Fatalf("updates = %#v, want visible text, completed read, and unresolved tool", updates)
	}
	if updates[3].notification.Update.ToolCall == nil {
		t.Fatalf("last update = %#v, want unresolved tool start", updates[3])
	}
}

func TestRetainedCapacityContinuationDoesNotRepeatCompletedTools(t *testing.T) {
	const sid acp.SessionId = "retained-capacity-completed-tools-test"
	clearRetainedCapacityCounters(sid)
	t.Cleanup(func() { clearRetainedCapacityCounters(sid) })
	updater := &mockUpdater{}
	a := &mockAgent{conn: updater}

	_, initialErr, handled := a.handleRetainedCapacity(context.Background(), sid, "/capacity-completed-tools")
	if !handled || initialErr == nil {
		t.Fatalf("initial request handled=%v error=%v, want capacity failure", handled, initialErr)
	}
	initialUpdates := updater.getUpdates()
	if len(initialUpdates) != 3 || initialUpdates[1].notification.Update.ToolCall == nil ||
		initialUpdates[2].notification.Update.ToolCallUpdate == nil {
		t.Fatalf("initial updates = %#v, want exactly one completed tool", initialUpdates)
	}

	_, continuationErr, handled := a.handleRetainedCapacity(context.Background(), sid,
		"Your previous turn stopped because the selected model was at capacity. Continue the unfinished request.")
	if !handled || continuationErr != nil {
		t.Fatalf("continuation handled=%v error=%v, want successful continuation", handled, continuationErr)
	}
	updates := updater.getUpdates()
	toolUpdates := 0
	for _, update := range updates {
		if update.notification.Update.ToolCall != nil || update.notification.Update.ToolCallUpdate != nil {
			toolUpdates++
		}
	}
	if toolUpdates != 2 || len(updates) != len(initialUpdates)+1 || updates[len(updates)-1].notification.Update.AgentMessageChunk == nil {
		t.Fatalf("continuation updates = %#v, want only a success message and no repeated tool", updates)
	}
}

func TestRetainedCapacityContinuationCanExhaustAcrossPrompts(t *testing.T) {
	const sid acp.SessionId = "retained-capacity-exhaust-continuation-test"
	clearRetainedCapacityCounters(sid)
	t.Cleanup(func() { clearRetainedCapacityCounters(sid) })
	a := &mockAgent{conn: &mockUpdater{}}
	if _, err, handled := a.handleRetainedCapacity(context.Background(), sid, "/capacity-exhaust"); !handled || err == nil {
		t.Fatalf("initial request handled=%v error=%v, want capacity failure", handled, err)
	}
	continuation := "Your previous turn stopped because the selected model was at capacity. Continue the unfinished request."
	for attempt := 2; attempt <= 9; attempt++ {
		if _, err, handled := a.handleRetainedCapacity(context.Background(), sid, continuation); !handled || err == nil {
			t.Fatalf("continuation attempt %d handled=%v error=%v, want capacity failure", attempt, handled, err)
		}
	}
	if _, err, handled := a.handleRetainedCapacity(context.Background(), sid, continuation); !handled || err != nil {
		t.Fatalf("final continuation handled=%v error=%v, want successful completion", handled, err)
	}
}
