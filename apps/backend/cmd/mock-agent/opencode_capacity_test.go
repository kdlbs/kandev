package main

import (
	"context"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestParseOpenCodeCapacityCmd(t *testing.T) {
	for _, test := range []struct {
		prompt string
		name   string
		fails  int
		ok     bool
	}{
		{prompt: "/opencode-capacity-after-tools", name: "after-tools", fails: 1, ok: true},
		{prompt: "/opencode-capacity-completed-tools", name: "completed-tools", fails: 1, ok: true},
		{prompt: "/opencode-capacity-retry:2", name: "retry", fails: 2, ok: true},
		{prompt: "/opencode-capacity-cancel", name: "cancel", fails: 9, ok: true},
		{prompt: "/opencode-capacity-exhaust", name: "exhaust", fails: 9, ok: true},
		{prompt: "<kandev-system>context</kandev-system>/opencode-capacity-retry", name: "retry", fails: 1, ok: true},
		{prompt: "/capacity-retry", ok: false},
		{prompt: "/opencode-capacity-unknown", ok: false},
	} {
		t.Run(test.prompt, func(t *testing.T) {
			got, ok := parseOpenCodeCapacityCmd(test.prompt)
			if ok != test.ok || (ok && (got.name != test.name || got.failTimes != test.fails)) {
				t.Fatalf("parseOpenCodeCapacityCmd(%q) = (%+v, %v)", test.prompt, got, ok)
			}
		})
	}
}

func TestOpenCodeCapacityFixtureUsesTheCapturedTerminalDiagnostic(t *testing.T) {
	const sid acp.SessionId = "opencode-capacity-terminal-diagnostic-test"
	clearRetainedCapacityCounters(sid)
	t.Cleanup(func() { clearRetainedCapacityCounters(sid) })
	a := &mockAgent{conn: &mockUpdater{}}

	_, err, handled := a.handleOpenCodeCapacity(context.Background(), sid, "/opencode-capacity-retry")
	if !handled || err == nil {
		t.Fatalf("handled=%v error=%v, want a capacity refusal", handled, err)
	}
	requestErr, ok := err.(*acp.RequestError)
	if !ok || requestErr.Code != -32603 {
		t.Fatalf("error = %#v, want an ACP application error", err)
	}
	if requestErr.Message != openCodeCapacityMessage {
		t.Fatalf("message = %q, want the captured OpenCode diagnostic", requestErr.Message)
	}
	if !strings.Contains(requestErr.Message, "[ttft_admission_refused] the cell is at capacity") ||
		!strings.HasSuffix(requestErr.Message, "(after 21 s)") {
		t.Fatalf("message = %q, want the bounded signature and relative hint", requestErr.Message)
	}
	if requestErr.Data != nil {
		t.Fatalf("data = %#v, want no mock capability marker", requestErr.Data)
	}
}

func TestOpenCodeCapacityFixtureEmitsNativeRetryMetadata(t *testing.T) {
	const sid acp.SessionId = "opencode-capacity-native-retry-test"
	clearRetainedCapacityCounters(sid)
	t.Cleanup(func() { clearRetainedCapacityCounters(sid) })
	updater := &mockUpdater{}
	a := &mockAgent{conn: updater}

	_, err, handled := a.handleOpenCodeCapacity(context.Background(), sid, "/opencode-capacity-completed-tools")
	if !handled || err == nil {
		t.Fatalf("handled=%v error=%v, want a capacity refusal", handled, err)
	}
	updates := updater.getUpdates()
	if len(updates) != 5 {
		t.Fatalf("updates = %#v, want retry metadata, cleared retry state, text, and one completed tool", updates)
	}
	retry := updates[0].notification.Update.SessionInfoUpdate
	if retry == nil {
		t.Fatalf("first update = %#v, want OpenCode retry metadata", updates[0])
	}
	meta, _ := retry.Meta["opencode/retry"].(map[string]any)
	if meta == nil || meta["attempt"] == nil {
		t.Fatalf("retry meta = %#v, want a native retry attempt", retry.Meta)
	}
	cleared := updates[1].notification.Update.SessionInfoUpdate
	if cleared == nil || cleared.Meta["opencode/retry"] != nil {
		t.Fatalf("second update = %#v, want the clearing null retry update", updates[1])
	}
	if updates[2].notification.Update.AgentMessageChunk == nil ||
		updates[3].notification.Update.ToolCall == nil || updates[4].notification.Update.ToolCallUpdate == nil {
		t.Fatalf("updates = %#v, want visible text and one completed tool", updates)
	}
}

func TestOpenCodeCapacityFixtureContinuationDoesNotRepeatCompletedTools(t *testing.T) {
	const sid acp.SessionId = "opencode-capacity-completed-tools-test"
	clearRetainedCapacityCounters(sid)
	t.Cleanup(func() { clearRetainedCapacityCounters(sid) })
	updater := &mockUpdater{}
	a := &mockAgent{conn: updater}

	_, initialErr, handled := a.handleOpenCodeCapacity(context.Background(), sid, "/opencode-capacity-completed-tools")
	if !handled || initialErr == nil {
		t.Fatalf("initial request handled=%v error=%v, want capacity failure", handled, initialErr)
	}
	initialUpdates := updater.getUpdates()
	completedTools := 0
	for _, update := range initialUpdates {
		if update.notification.Update.ToolCall != nil {
			completedTools++
		}
	}
	if completedTools != 1 {
		t.Fatalf("initial tool calls = %d, want exactly one completed effect", completedTools)
	}

	_, continuationErr, handled := a.handleOpenCodeCapacity(context.Background(), sid,
		"Your previous turn stopped because the selected model was at capacity. Continue the unfinished request.")
	if !handled || continuationErr != nil {
		t.Fatalf("continuation handled=%v error=%v, want successful continuation", handled, continuationErr)
	}
	updates := updater.getUpdates()
	last := updates[len(updates)-1].notification.Update
	if last.AgentMessageChunk == nil {
		t.Fatalf("last update = %#v, want continuation text", last)
	}
	if got := len(updates) - len(initialUpdates); got != 3 {
		t.Fatalf("continuation emitted %d updates, want retry metadata and text only", got)
	}
}

func TestOpenCodeCapacityFixtureExhaustsThenRecovers(t *testing.T) {
	const sid acp.SessionId = "opencode-capacity-exhaust-test"
	clearRetainedCapacityCounters(sid)
	t.Cleanup(func() { clearRetainedCapacityCounters(sid) })
	a := &mockAgent{conn: &mockUpdater{}}

	if _, err, handled := a.handleOpenCodeCapacity(context.Background(), sid, "/opencode-capacity-exhaust"); !handled || err == nil {
		t.Fatalf("initial handled=%v error=%v, want capacity failure", handled, err)
	}
	for attempt := 2; attempt <= 9; attempt++ {
		if _, err, handled := a.handleOpenCodeCapacity(context.Background(), sid, "/opencode-capacity-exhaust"); !handled || err == nil {
			t.Fatalf("attempt %d handled=%v error=%v, want capacity failure", attempt, handled, err)
		}
	}
	if _, err, handled := a.handleOpenCodeCapacity(context.Background(), sid, "/opencode-capacity-exhaust"); !handled || err != nil {
		t.Fatalf("recovery handled=%v error=%v, want success after the episode", handled, err)
	}
}
