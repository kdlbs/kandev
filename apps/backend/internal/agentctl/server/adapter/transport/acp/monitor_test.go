package acp

import (
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// monitorMeta builds the `_meta.claudeCode.toolName=Monitor` payload Claude-acp
// attaches to every Monitor tool_call notification.
func monitorMeta() map[string]any {
	return map[string]any{"claudeCode": map[string]any{"toolName": monitorToolName}}
}

// nonMonitorMeta builds a `_meta.claudeCode.toolName=Bash` payload — used to
// confirm the Monitor recognizers don't fire for unrelated tools.
func nonMonitorMeta() map[string]any {
	return map[string]any{"claudeCode": map[string]any{"toolName": "Bash"}}
}

// seedMonitor registers a Monitor as if its registration update had arrived.
// Returns the toolCallID for use in subsequent assertions.
func seedMonitor(t *testing.T, a *Adapter, sessionID, taskID, toolCallID string) {
	t.Helper()
	a.trackMonitor(sessionID, taskID, toolCallID)
	if got, ok := a.lookupMonitorByTaskID(sessionID, taskID); !ok || got != toolCallID {
		t.Fatalf("seedMonitor: lookup after track failed (ok=%v, got=%q)", ok, got)
	}
}

// --- recognizeMonitorRegistration ---

func TestRecognizeMonitorRegistration_HappyPath(t *testing.T) {
	taskID, ok := recognizeMonitorRegistration(monitorMeta(),
		"Monitor started (task abc123, timeout 60000ms). You will be notified on each event.")
	if !ok || taskID != "abc123" {
		t.Errorf("recognize = (%q, %v), want (abc123, true)", taskID, ok)
	}
}

func TestRecognizeMonitorRegistration_WrongToolName(t *testing.T) {
	if _, ok := recognizeMonitorRegistration(nonMonitorMeta(),
		"Monitor started (task abc123, timeout 60000ms)."); ok {
		t.Error("recognized non-Monitor meta as Monitor registration")
	}
}

func TestRecognizeMonitorRegistration_NotABanner(t *testing.T) {
	if _, ok := recognizeMonitorRegistration(monitorMeta(), "some other output"); ok {
		t.Error("recognized arbitrary string as registration banner")
	}
}

func TestRecognizeMonitorRegistration_NonStringRawOutput(t *testing.T) {
	if _, ok := recognizeMonitorRegistration(monitorMeta(),
		map[string]any{"output": "Monitor started (task abc, …)"}); ok {
		t.Error("recognized object rawOutput as banner")
	}
}

// --- extractMonitorEvents ---

func TestExtractMonitorEvents_SingleEvent(t *testing.T) {
	in := "Human: <task-notification>\n<task-id>t1</task-id>\n<event>event-A</event>\n</task-notification>"
	cleaned, events := extractMonitorEvents(in)
	if cleaned != "" {
		t.Errorf("cleaned = %q, want empty", cleaned)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].TaskID != "t1" || events[0].Body != "event-A" {
		t.Errorf("event = %+v, want {t1, event-A}", events[0])
	}
}

func TestExtractMonitorEvents_MultipleEnvelopes(t *testing.T) {
	in := "<task-notification><task-id>t1</task-id><event>a</event></task-notification>" +
		" some text " +
		"<task-notification><task-id>t1</task-id><event>b</event></task-notification>"
	cleaned, events := extractMonitorEvents(in)
	if !strings.Contains(cleaned, "some text") {
		t.Errorf("cleaned = %q, want it to contain 'some text'", cleaned)
	}
	if strings.Contains(cleaned, "task-notification") {
		t.Errorf("cleaned still contains envelope: %q", cleaned)
	}
	if len(events) != 2 || events[0].Body != "a" || events[1].Body != "b" {
		t.Errorf("events = %+v, want [{t1,a},{t1,b}]", events)
	}
}

func TestExtractMonitorEvents_NoEnvelopeReturnsInputUnchanged(t *testing.T) {
	in := "regular assistant text without envelopes"
	cleaned, events := extractMonitorEvents(in)
	if cleaned != in {
		t.Errorf("cleaned mutated text without envelopes: %q", cleaned)
	}
	if events != nil {
		t.Errorf("events = %v, want nil", events)
	}
}

func TestExtractMonitorEvents_EmptyEventBody(t *testing.T) {
	in := "<task-notification><task-id>t1</task-id><event></event></task-notification>"
	_, events := extractMonitorEvents(in)
	if len(events) != 1 || events[0].Body != "" {
		t.Errorf("events = %+v, want one with empty body", events)
	}
}

func TestExtractMonitorEvents_BodyContainingAngleBrackets(t *testing.T) {
	// Real-world scripts emit lines containing `<` (XML-ish errors, shell
	// redirection like `< /dev/null`, ANSI fragments). The regex must not
	// abort on the first `<` it sees inside the event body; otherwise the
	// whole envelope leaks to the chat as raw assistant text.
	in := "<task-notification><task-id>t1</task-id><event><error>build failed: exit < 1</error></event></task-notification>"
	_, events := extractMonitorEvents(in)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1 (regex should accept '<' in body)", len(events))
	}
	if events[0].Body != "<error>build failed: exit < 1</error>" {
		t.Errorf("event body = %q, want literal '<error>...' carried through", events[0].Body)
	}
}

// --- isMonitorHumanEcho ---

func TestIsMonitorHumanEcho_Variants(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"bare prefix", "Human:", true},
		{"prefix with whitespace", "  Human:   ", true},
		{"prefix with partial open tag", "Human: <task-noti", true},
		{"genuine assistant text mentioning Human:", "Human: he said yes.", false},
		{"empty", "", false},
		{"normal assistant text", "Sure, here's the result.", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isMonitorHumanEcho(tc.text); got != tc.want {
				t.Errorf("isMonitorHumanEcho(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// --- routeMonitorEvents (integration with adapter state) ---

func TestRouteMonitorEvents_EmitsSyntheticUpdateAndStripsEnvelope(t *testing.T) {
	a := newTestAdapter()
	seedMonitor(t, a, "s1", "t1", "tc-monitor")

	cleaned := a.routeMonitorEvents("s1",
		"<task-notification><task-id>t1</task-id><event>event-A</event></task-notification>")
	if strings.TrimSpace(cleaned) != "" {
		t.Errorf("cleaned = %q, want empty (envelope-only chunk)", cleaned)
	}

	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	ev := events[0]
	if ev.Type != streams.EventTypeToolUpdate {
		t.Errorf("Type = %q, want %q", ev.Type, streams.EventTypeToolUpdate)
	}
	if ev.ToolCallID != "tc-monitor" {
		t.Errorf("ToolCallID = %q, want tc-monitor", ev.ToolCallID)
	}
	if ev.ToolStatus != "in_progress" {
		t.Errorf("ToolStatus = %q, want in_progress", ev.ToolStatus)
	}
	if len(ev.ToolCallContents) != 1 || ev.ToolCallContents[0].Content == nil ||
		ev.ToolCallContents[0].Content.Text != "event-A" {
		t.Errorf("contents = %+v, want one text content 'event-A'", ev.ToolCallContents)
	}
}

func TestRouteMonitorEvents_UnknownTaskIDLeavesTextIntact(t *testing.T) {
	a := newTestAdapter()
	// no Monitor registered
	in := "<task-notification><task-id>tX</task-id><event>orphan</event></task-notification>"
	cleaned := a.routeMonitorEvents("s1", in)
	// extractMonitorEvents still strips the matched envelope from the cleaned
	// text (the envelope is meaningless to the user even when un-routable),
	// but no synthetic update is emitted.
	if strings.Contains(cleaned, "task-notification") {
		t.Errorf("cleaned still contains envelope: %q", cleaned)
	}
	if got := drainEvents(a); len(got) != 0 {
		t.Errorf("got %d events for unknown taskID, want 0", len(got))
	}
}

func TestRouteMonitorEvents_NoEnvelopeShortCircuits(t *testing.T) {
	a := newTestAdapter()
	in := "regular assistant text"
	cleaned := a.routeMonitorEvents("s1", in)
	if cleaned != in {
		t.Errorf("cleaned = %q, want unchanged", cleaned)
	}
	if got := drainEvents(a); len(got) != 0 {
		t.Errorf("got %d events for no-envelope text, want 0", len(got))
	}
}

// --- convertToolCallResultUpdate Monitor registration override ---

func TestConvertToolCallResultUpdate_MonitorRegistrationOverridesCompleted(t *testing.T) {
	a := newTestAdapter()
	completed := acp.ToolCallStatus("completed")
	tcu := &acp.SessionToolCallUpdate{
		ToolCallId: "tc-monitor",
		Status:     &completed,
		Meta:       monitorMeta(),
		RawOutput:  "Monitor started (task taskZZ, timeout 60000ms). You will be notified.",
	}

	ev := a.convertToolCallResultUpdate("s1", tcu)
	if ev == nil {
		t.Fatal("expected event, got nil")
	}
	if ev.ToolStatus != "in_progress" {
		t.Errorf("ToolStatus = %q, want in_progress (registration banner must override)", ev.ToolStatus)
	}
	// Side effect: activeMonitors records taskID -> toolCallID
	if got, ok := a.lookupMonitorByTaskID("s1", "taskZZ"); !ok || got != "tc-monitor" {
		t.Errorf("activeMonitors lookup = (%q, %v), want (tc-monitor, true)", got, ok)
	}
}

func TestConvertToolCallResultUpdate_RealCompletedStaysComplete(t *testing.T) {
	a := newTestAdapter()
	completed := acp.ToolCallStatus("completed")
	// rawOutput is NOT a registration banner, so we should not override.
	tcu := &acp.SessionToolCallUpdate{
		ToolCallId: "tc-bash",
		Status:     &completed,
		Meta:       monitorMeta(), // even Monitor-tagged completes can occur (e.g. final exit)
		RawOutput:  "exit 0\n",
	}
	ev := a.convertToolCallResultUpdate("s1", tcu)
	if ev == nil {
		t.Fatal("expected event")
	}
	if ev.ToolStatus != "complete" {
		t.Errorf("ToolStatus = %q, want complete (non-banner output must not override)", ev.ToolStatus)
	}
}

// TestConvertToolCallResultUpdate_MonitorRegistrationSurvivesNormalize is a
// regression guard: NormalizeToolResult writes the rawOutput banner string
// into the Generic payload's Output field, which would shadow the
// `output.monitor` view the frontend uses to detect Monitor cards. The
// Monitor seed must run AFTER NormalizeToolResult so the synthetic
// `{monitor: …}` wrapper is the final value.
func TestConvertToolCallResultUpdate_MonitorRegistrationSurvivesNormalize(t *testing.T) {
	a := newTestAdapter()

	// First the initial pending tool_call lands in activeToolCalls so
	// convertToolCallResultUpdate has a Generic payload to mutate.
	tc := &acp.SessionUpdateToolCall{
		ToolCallId: "tc-monitor",
		Title:      monitorToolName,
		Kind:       acp.ToolKind("other"),
		Meta:       monitorMeta(),
		RawInput:   map[string]any{"command": "tail -f /var/log/x"},
	}
	if ev := a.convertToolCallUpdate("s1", tc); ev == nil {
		t.Fatalf("seed: convertToolCallUpdate returned nil")
	}

	completed := acp.ToolCallStatus("completed")
	tcu := &acp.SessionToolCallUpdate{
		ToolCallId: "tc-monitor",
		Status:     &completed,
		Meta:       monitorMeta(),
		RawOutput:  "Monitor started (task realTaskId, timeout 60000ms). You will be notified.",
	}

	ev := a.convertToolCallResultUpdate("s1", tcu)
	if ev == nil {
		t.Fatal("expected event, got nil")
	}
	if ev.NormalizedPayload == nil || ev.NormalizedPayload.Generic() == nil {
		t.Fatalf("expected Generic payload, got %+v", ev.NormalizedPayload)
	}
	out, ok := ev.NormalizedPayload.Generic().Output.(map[string]any)
	if !ok {
		t.Fatalf("Generic.Output = %v (%T), want map (the {monitor: …} wrapper, not the raw banner string)",
			ev.NormalizedPayload.Generic().Output, ev.NormalizedPayload.Generic().Output)
	}
	monitor, ok := out["monitor"].(map[string]any)
	if !ok {
		t.Fatalf("Generic.Output[monitor] missing or wrong type — frontend would fall back to generic tool_call rendering")
	}
	if monitor["task_id"] != "realTaskId" {
		t.Errorf("monitor.task_id = %v, want realTaskId", monitor["task_id"])
	}
	if monitor["command"] != "tail -f /var/log/x" {
		t.Errorf("monitor.command = %v, want it carried over from initial tool_call", monitor["command"])
	}
}

// --- sweepMonitorsOnPromptEnd ---

func TestSweepMonitorsOnPromptEnd_EmitsCompleteAndClears(t *testing.T) {
	a := newTestAdapter()
	seedMonitor(t, a, "s1", "t1", "tc-1")
	seedMonitor(t, a, "s1", "t2", "tc-2")

	a.sweepMonitorsOnPromptEnd("s1")

	events := drainEvents(a)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	for _, ev := range events {
		if ev.ToolStatus != "complete" {
			t.Errorf("ToolStatus = %q, want complete", ev.ToolStatus)
		}
	}
	if _, ok := a.lookupMonitorByTaskID("s1", "t1"); ok {
		t.Error("activeMonitors not cleared after sweep")
	}
}

// TestConvertToolCallResultUpdate_AgentEmittedMonitorEndPreservesView is a
// regression guard for an E2E failure on the first PR push: when the agent
// emits its own terminal `tool_call_update` for a tracked Monitor (status
// completed, plain rawOutput like "Monitor exited"), `NormalizeToolResult`
// would clobber `Generic.Output` with that string and the frontend's
// MonitorMessage matcher (which checks for `output.monitor`) would no
// longer fire. Frontend would fall back to the generic tool_call card.
func TestConvertToolCallResultUpdate_AgentEmittedMonitorEndPreservesView(t *testing.T) {
	a := newTestAdapter()

	// Initial pending tool_call so activeToolCalls has a Generic payload.
	tc := &acp.SessionUpdateToolCall{
		ToolCallId: "tc-monitor",
		Title:      monitorToolName,
		Kind:       acp.ToolKind("other"),
		Meta:       monitorMeta(),
		RawInput:   map[string]any{"command": "tail -f /var/log/x"},
	}
	if ev := a.convertToolCallUpdate("s1", tc); ev == nil {
		t.Fatalf("seed tool_call returned nil")
	}

	// Registration update — establishes the Monitor view and tracks taskID.
	completed := acp.ToolCallStatus("completed")
	registerTcu := &acp.SessionToolCallUpdate{
		ToolCallId: "tc-monitor",
		Status:     &completed,
		Meta:       monitorMeta(),
		RawOutput:  "Monitor started (task taskZZ, timeout 60000ms).",
	}
	if ev := a.convertToolCallResultUpdate("s1", registerTcu); ev == nil {
		t.Fatalf("registration update returned nil")
	}

	// Agent-emitted Monitor end — what mock-agent's e2e:monitor_end produces
	// and what real claude-acp emits when a Monitor script exits naturally.
	endTcu := &acp.SessionToolCallUpdate{
		ToolCallId: "tc-monitor",
		Status:     &completed,
		Meta:       monitorMeta(),
		RawOutput:  "Monitor exited",
	}
	ev := a.convertToolCallResultUpdate("s1", endTcu)
	if ev == nil {
		t.Fatal("expected event for end update, got nil")
	}
	if ev.NormalizedPayload == nil || ev.NormalizedPayload.Generic() == nil {
		t.Fatalf("expected Generic payload to survive, got %+v", ev.NormalizedPayload)
	}
	out, ok := ev.NormalizedPayload.Generic().Output.(map[string]any)
	if !ok {
		t.Fatalf("Generic.Output = %v (%T), want the {monitor: …} wrapper preserved through agent-emitted end",
			ev.NormalizedPayload.Generic().Output, ev.NormalizedPayload.Generic().Output)
	}
	monitor, ok := out["monitor"].(map[string]any)
	if !ok {
		t.Fatal("Generic.Output[monitor] missing — agent-emitted end stomped the view")
	}
	if monitor["ended"] != true {
		t.Errorf("monitor.ended = %v, want true (terminal update should mark ended)", monitor["ended"])
	}
	// The tracked entry should be dropped so the prompt-end sweep does not
	// re-emit a terminal event for the same toolCallID.
	if a.isTrackedMonitor("s1", "tc-monitor") {
		t.Error("Monitor still tracked after agent-emitted end — sweep would double-emit")
	}
}

// TestSweepMonitorsOnPromptEnd_NotDoubleCancelledByActiveToolCalls is a
// regression guard for a real wire-bug discovered in CI: when the parent
// prompt naturally ends, `cancelActiveToolCalls` would blanket-cancel every
// entry in `activeToolCalls` (including the Monitor's tool call) before
// `sweepMonitorsOnPromptEnd` ran, leaving the frontend with two
// conflicting terminal events for the same Monitor. Monitor entries must
// be skipped in the cancel loop so the sweep emits the single
// authoritative "Monitor exited" event.
func TestSweepMonitorsOnPromptEnd_NotDoubleCancelledByActiveToolCalls(t *testing.T) {
	a := newTestAdapter()

	// Land an initial Monitor tool_call so it sits in activeToolCalls.
	tc := &acp.SessionUpdateToolCall{
		ToolCallId: "tc-monitor",
		Title:      monitorToolName,
		Kind:       acp.ToolKind("other"),
		Meta:       monitorMeta(),
		RawInput:   map[string]any{"command": "tail -f log"},
	}
	if ev := a.convertToolCallUpdate("s1", tc); ev == nil {
		t.Fatalf("seed tool_call returned nil")
	}
	a.trackMonitor("s1", "task-X", "tc-monitor")
	drainEvents(a) // ignore the initial events

	a.cancelActiveToolCalls("s1")
	a.sweepMonitorsOnPromptEnd("s1")

	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1 (the Monitor sweep's terminal event only)", len(events))
	}
	if events[0].ToolCallID != "tc-monitor" {
		t.Errorf("event ToolCallID = %q, want tc-monitor", events[0].ToolCallID)
	}
	if events[0].ToolStatus != "complete" {
		t.Errorf("event ToolStatus = %q, want complete (cancelActiveToolCalls must skip Monitors)", events[0].ToolStatus)
	}
	if events[0].NormalizedPayload == nil {
		t.Errorf("event NormalizedPayload is nil — sweep lost payload (activeToolCalls drained too early)")
	}
}

func TestSweepMonitorsOnReplayEnd_EmitsCancelledWithRestartNote(t *testing.T) {
	a := newTestAdapter()
	seedMonitor(t, a, "s1", "t1", "tc-1")

	a.sweepMonitorsOnReplayEnd("s1")

	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	ev := events[0]
	if ev.ToolStatus != "cancelled" {
		t.Errorf("ToolStatus = %q, want cancelled", ev.ToolStatus)
	}
	if len(ev.ToolCallContents) == 0 || ev.ToolCallContents[0].Content == nil ||
		!strings.Contains(ev.ToolCallContents[0].Content.Text, "session restart") {
		t.Errorf("contents = %+v, want a 'session restart' note", ev.ToolCallContents)
	}
}

// --- captureReplayMonitor (replay rebuild) ---

func TestCaptureReplayMonitor_RebuildsFromToolCallAndUpdate(t *testing.T) {
	a := newTestAdapter()

	// Replay the initial tool_call (we don't yet know taskID).
	tc := &acp.SessionUpdateToolCall{
		ToolCallId: "tc-replay",
		Title:      monitorToolName,
		Kind:       acp.ToolKind("other"),
		Meta:       monitorMeta(),
		RawInput:   map[string]any{"command": "tail -f /var/log/x"},
	}
	a.captureReplayMonitor("s1", acp.SessionUpdate{ToolCall: tc})

	// Replay the registration banner — taskID becomes known.
	completed := acp.ToolCallStatus("completed")
	tcu := &acp.SessionToolCallUpdate{
		ToolCallId: "tc-replay",
		Status:     &completed,
		Meta:       monitorMeta(),
		RawOutput:  "Monitor started (task realTask99, timeout 60000ms).",
	}
	a.captureReplayMonitor("s1", acp.SessionUpdate{ToolCallUpdate: tcu})

	if got, ok := a.lookupMonitorByTaskID("s1", "realTask99"); !ok || got != "tc-replay" {
		t.Errorf("after replay: lookup = (%q, %v), want (tc-replay, true)", got, ok)
	}
}

func TestCaptureReplayMonitor_TerminalUpdateRemovesFromMap(t *testing.T) {
	a := newTestAdapter()
	seedMonitor(t, a, "s1", "tX", "tc-9")

	cancelled := acp.ToolCallStatus("cancelled")
	tcu := &acp.SessionToolCallUpdate{
		ToolCallId: "tc-9",
		Status:     &cancelled,
		Meta:       monitorMeta(),
	}
	a.captureReplayMonitor("s1", acp.SessionUpdate{ToolCallUpdate: tcu})

	if _, ok := a.lookupMonitorByTaskID("s1", "tX"); ok {
		t.Error("terminal replay update did not drop Monitor from map")
	}
}

// --- convertMessageChunk integration ---

func TestConvertMessageChunk_MonitorEnvelopeStrippedAndRouted(t *testing.T) {
	a := newTestAdapter()
	seedMonitor(t, a, "s1", "t1", "tc-monitor")

	chunk := acp.TextBlock(
		"Human: <task-notification><task-id>t1</task-id><event>line-1</event></task-notification>",
	)
	ev := a.convertMessageChunk("s1", chunk, "assistant")
	if ev != nil {
		t.Errorf("expected nil (envelope-only chunk should drop), got %+v", ev)
	}

	// One synthetic event should have been routed to the Monitor's tool card.
	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("got %d routed events, want 1", len(events))
	}
	if events[0].ToolCallID != "tc-monitor" {
		t.Errorf("ToolCallID = %q, want tc-monitor", events[0].ToolCallID)
	}
}

func TestConvertMessageChunk_PlainAssistantTextPassesThrough(t *testing.T) {
	a := newTestAdapter()
	chunk := acp.TextBlock("Sure, here's the result.")
	ev := a.convertMessageChunk("s1", chunk, "assistant")
	if ev == nil {
		t.Fatal("expected event for plain text, got nil")
	}
	if ev.Text != "Sure, here's the result." {
		t.Errorf("Text = %q, want unchanged", ev.Text)
	}
}

func TestConvertMessageChunk_HumanEchoOnlyChunkDropped(t *testing.T) {
	a := newTestAdapter()
	chunk := acp.TextBlock("Human:")
	ev := a.convertMessageChunk("s1", chunk, "assistant")
	if ev != nil {
		t.Errorf("expected nil for orphan Human: prefix, got %+v", ev)
	}
}

func TestConvertMessageChunk_UserRoleSkipsMonitorRouting(t *testing.T) {
	a := newTestAdapter()
	seedMonitor(t, a, "s1", "t1", "tc-monitor")

	chunk := acp.TextBlock(
		"<task-notification><task-id>t1</task-id><event>x</event></task-notification>",
	)
	ev := a.convertMessageChunk("s1", chunk, "user")
	if ev == nil {
		t.Fatal("expected event for user role, got nil")
	}
	// User text must reach the chat untouched — the parser should only run for assistant role.
	if !strings.Contains(ev.Text, "task-notification") {
		t.Errorf("user text was scrubbed: %q", ev.Text)
	}
	if got := drainEvents(a); len(got) != 0 {
		t.Errorf("got %d routed events for user role, want 0", len(got))
	}
}
