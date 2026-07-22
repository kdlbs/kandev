package main

import (
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// TestParseBackgroundDuration pins the /background argument parsing, including
// the regression Copilot flagged on PR #3: a unit-bearing value like "1m" must
// not be mangled into "1ms" by the bare-seconds fallback.
func TestParseBackgroundDuration(t *testing.T) {
	const def = 8 * time.Second
	cases := []struct {
		name string
		cmd  string
		want time.Duration
	}{
		{"no argument uses default", "/background", def},
		{"bare number is seconds", "/background 12", 12 * time.Second},
		{"explicit seconds", "/background 20s", 20 * time.Second},
		{"explicit minutes (regression: not 1ms)", "/background 1m", time.Minute},
		{"explicit hours", "/background 2h", 2 * time.Hour},
		{"explicit milliseconds", "/background 500ms", 500 * time.Millisecond},
		{"unparseable falls back to default", "/background soon", def},
		{"zero falls back to default", "/background 0", def},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseBackgroundDuration(tc.cmd, def); got != tc.want {
				t.Fatalf("parseBackgroundDuration(%q) = %v, want %v", tc.cmd, got, tc.want)
			}
		})
	}
}

func TestDetachedBackgroundLifecycleFrames(t *testing.T) {
	e, updates := newTestEmitter()
	e.launchAsyncSubagentTool("detached-1", "Detached work", "Keep working", "general-purpose")
	e.completeDetachedWork()

	got := updates.getUpdates()
	if len(got) != 3 {
		t.Fatalf("updates = %d, want launch, async acknowledgement, and completion", len(got))
	}
	if got[0].notification.Update.ToolCall == nil {
		t.Fatal("first update must register the subagent tool")
	}
	launch := got[1].notification.Update.ToolCallUpdate
	if launch == nil {
		t.Fatal("second update must acknowledge the async launch")
	}
	response := launch.Meta["claudeCode"].(map[string]any)["toolResponse"].(map[string]any)
	if response[subagentKeyStatus] != subagentStatusAsync || response["isAsync"] != true {
		t.Fatalf("async launch response = %#v", response)
	}

	usage := got[2].notification.Update.UsageUpdate
	if usage == nil {
		t.Fatal("third update must be a usage lifecycle boundary")
	}
	origin := usage.Meta[claudeOriginMetaKey].(map[string]any)
	if origin["kind"] != claudeOriginTaskNotification {
		t.Fatalf("completion origin = %#v", origin)
	}
	if got[2].notification.SessionId != acp.SessionId("test-session") {
		t.Fatalf("completion session = %q", got[2].notification.SessionId)
	}
}

func TestAsyncSubagentForegroundFramesMatchClaudeOrdering(t *testing.T) {
	e, updates := newTestEmitter()
	emitAsyncSubagentLifecycle(e, "/async-subagent-teardown", false)

	got := updates.getUpdates()
	if len(got) != 5 {
		t.Fatalf("updates = %d, want launch, async acknowledgement, idle, thought, and message", len(got))
	}
	if got[0].notification.Update.ToolCall == nil {
		t.Fatal("first update must start the Agent tool")
	}
	launch := got[1].notification.Update.ToolCallUpdate
	if launch == nil {
		t.Fatal("second update must acknowledge the async launch")
	}
	response := launch.Meta["claudeCode"].(map[string]any)["toolResponse"].(map[string]any)
	if response["agentId"] != asyncSubagentAgentID || response[subagentKeyStatus] != subagentStatusAsync {
		t.Fatalf("async launch response = %#v", response)
	}
	idle := got[2].notification.Update.UsageUpdate
	if idle == nil {
		t.Fatal("third update must be the foreground-idle usage boundary")
	}
	origin := idle.Meta[claudeOriginMetaKey].(map[string]any)
	if origin["kind"] != claudeOriginHuman {
		t.Fatalf("idle origin = %#v", origin)
	}
	if !isThoughtUpdate(got[3]) {
		t.Fatal("fourth update must be final same-prompt thinking")
	}
	if !isTextUpdate(got[4]) || getTextContent(got[4]) != "Foreground response after async launch." {
		t.Fatalf("fifth update must be final same-prompt assistant output, got %#v", got[4])
	}
}
