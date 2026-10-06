package main

import (
	"context"
	"os"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestParseRetainedCapacityCmd(t *testing.T) {
	for _, test := range []struct {
		prompt string
		name   string
		fails  int
		ok     bool
	}{
		{prompt: "/capacity-after-tools", name: "after-tools", fails: 1, ok: true},
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

func TestHandleRetainedCapacityAfterToolsEmitsVisibleReadBeforeFailure(t *testing.T) {
	const sid acp.SessionId = "retained-capacity-tools-test"
	_ = os.Remove(retainedCapacityCounterPath(sid, "after-tools"))
	t.Cleanup(func() { _ = os.Remove(retainedCapacityCounterPath(sid, "after-tools")) })
	updater := &mockUpdater{}
	a := &mockAgent{conn: updater}
	_, err, handled := a.handleRetainedCapacity(context.Background(), sid, "/capacity-after-tools")
	if !handled || err == nil {
		t.Fatalf("handled=%v error=%v, want marked provider failure", handled, err)
	}
	updates := updater.getUpdates()
	if len(updates) != 3 || updates[0].notification.Update.AgentMessageChunk == nil ||
		updates[1].notification.Update.ToolCall == nil || updates[2].notification.Update.ToolCallUpdate == nil {
		t.Fatalf("updates = %#v, want visible text plus completed read tool", updates)
	}
}
