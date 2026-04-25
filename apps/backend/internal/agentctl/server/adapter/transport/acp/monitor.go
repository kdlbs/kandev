package acp

import (
	"regexp"
	"strings"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// monitorToolName is the literal toolName Claude-acp tags Monitor tool calls with
// in `_meta.claudeCode.toolName`. Used to recognize Monitor across the lifecycle.
const monitorToolName = "Monitor"

// monitorRegistrationOutputPrefix identifies the rawOutput banner Claude-acp
// emits when a Monitor registers (~1s after start). The wrapper sets status to
// "completed" on this update, but the Monitor itself is only just starting —
// we override that status downstream.
const monitorRegistrationOutputPrefix = "Monitor started (task "

// monitorRegistrationRE captures the taskId out of the registration banner.
// Banner format: `Monitor started (task <taskId>, timeout <ms>ms)…`
var monitorRegistrationRE = regexp.MustCompile(`^Monitor started \(task ([a-zA-Z0-9_-]+),`)

// monitorTaskNotifRE matches the `<task-notification>` envelope claude-agent-acp's
// model auto-completes when a Monitor event fires. The model leaks these as
// agent_message_chunk text because the wrapper suppresses the corresponding
// user_message_chunk. Capture groups: 1=taskId, 2=event body.
//
// Pattern is intentionally permissive: the outer `Human:` prefix is optional
// (some chunks split across multiple deltas), we match across newlines, and
// the event body uses lazy `(.*?)` rather than `[^<]*` so script lines that
// contain `<` characters (e.g. `<error>build failed</error>`, ANSI escape
// fragments, shell redirection text) still match instead of leaking the
// raw envelope into the chat.
var monitorTaskNotifRE = regexp.MustCompile(
	`(?s)(?:Human:\s*)?<task-notification>\s*<task-id>([^<]+)</task-id>.*?<event>(.*?)</event>\s*</task-notification>`,
)

// monitorHumanEchoRE matches an orphan "Human:" prefix with no closing
// `</task-notification>` — i.e. the model started echoing an envelope but the
// chunk got cut off. Conservative pattern: drop only when the chunk is just the
// prefix (possibly with a partial `<…` opener), so genuine assistant messages
// that happen to mention "Human:" survive.
var monitorHumanEchoRE = regexp.MustCompile(`^Human:\s*(<[^>]*)?\s*$`)

// recognizeMonitorRegistration inspects a tool_call_update for the Monitor
// registration banner. Returns the parsed taskID and true when the update
// represents a registration (not a real completion). The caller is expected to
// override the outgoing status from "completed" to "in_progress" so the
// Monitor card stays open in the UI.
func recognizeMonitorRegistration(meta map[string]any, rawOutput any) (string, bool) {
	if !isMonitorMeta(meta) {
		return "", false
	}
	out, ok := rawOutput.(string)
	if !ok || !strings.HasPrefix(out, monitorRegistrationOutputPrefix) {
		return "", false
	}
	matches := monitorRegistrationRE.FindStringSubmatch(out)
	if len(matches) != 2 {
		return "", false
	}
	return matches[1], true
}

// isMonitorMeta returns true if the ACP `_meta.claudeCode.toolName` field
// equals "Monitor". The meta map is shaped untyped over the wire (`{claudeCode:
// {toolName: …}}`), so we walk it defensively.
func isMonitorMeta(meta map[string]any) bool {
	if meta == nil {
		return false
	}
	cc, ok := meta["claudeCode"].(map[string]any)
	if !ok {
		return false
	}
	name, _ := cc["toolName"].(string)
	return name == monitorToolName
}

// recognizeMonitorTaskCall inspects a `tool_call` notification's metadata and
// rawInput. Returns the watched command (best-effort) when the call is a
// Monitor invocation, plus true. Used during replay to seed activeMonitors
// before the matching registration update arrives.
func recognizeMonitorTaskCall(tc *acp.SessionUpdateToolCall) (string, bool) {
	if tc == nil || !isMonitorMeta(tc.Meta) {
		return "", false
	}
	cmd := ""
	if input, ok := tc.RawInput.(map[string]any); ok {
		if c, ok := input["command"].(string); ok {
			cmd = c
		}
	}
	return cmd, true
}

// monitorCommandFromPayload extracts the watched command string from a
// Generic tool payload's Input field. The ACP normalizer stores the entire
// args map (including a `raw_input` key) under Input, so the command lives
// at `Input.raw_input.command` for Generic tools — not at a top-level key.
// Returns empty string when the path is missing or the wrong shape.
func monitorCommandFromPayload(payload *streams.NormalizedPayload) string {
	if payload == nil || payload.Generic() == nil {
		return ""
	}
	args, ok := payload.Generic().Input.(map[string]any)
	if !ok {
		return ""
	}
	rawInput, ok := args["raw_input"].(map[string]any)
	if !ok {
		// Fallback: some agents pass command at the top level.
		cmd, _ := args["command"].(string)
		return cmd
	}
	cmd, _ := rawInput["command"].(string)
	return cmd
}

// isTrackedMonitor returns true if any taskID -> toolCallID mapping in the
// session's activeMonitors entry equals the given toolCallID. Used by
// convertToolCallResultUpdate to recognize agent-emitted terminal updates
// for Monitors that we already know about, so the rawOutput string doesn't
// stomp the structured Monitor view in Generic.Output.
func (a *Adapter) isTrackedMonitor(sessionID, toolCallID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, tcID := range a.activeMonitors[sessionID] {
		if tcID == toolCallID {
			return true
		}
	}
	return false
}

// dropMonitorByToolCallIDLocked is the same as dropMonitorByToolCallID but
// the caller already holds Adapter.mu. Used inside convertToolCallResultUpdate
// where the lock is already held for the duration of the payload update
// block.
func (a *Adapter) dropMonitorByToolCallIDLocked(sessionID, toolCallID string) {
	monitors := a.activeMonitors[sessionID]
	if monitors == nil {
		return
	}
	for taskID, tcID := range monitors {
		if tcID == toolCallID {
			delete(monitors, taskID)
		}
	}
	if len(monitors) == 0 {
		delete(a.activeMonitors, sessionID)
	}
}

// trackMonitor records a registered Monitor against a session.
func (a *Adapter) trackMonitor(sessionID, taskID, toolCallID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	monitors := a.activeMonitors[sessionID]
	if monitors == nil {
		monitors = make(map[string]string)
		a.activeMonitors[sessionID] = monitors
	}
	monitors[taskID] = toolCallID
}

// lookupMonitorByTaskID resolves a taskID to its toolCallID for a session.
// Returns the toolCallID and true if the Monitor is currently tracked.
func (a *Adapter) lookupMonitorByTaskID(sessionID, taskID string) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	monitors := a.activeMonitors[sessionID]
	if monitors == nil {
		return "", false
	}
	tcID, ok := monitors[taskID]
	return tcID, ok
}

// takeActiveMonitors atomically removes and returns the active Monitor map
// for a session. Used by the prompt-end sweep and the post-replay restart
// sweep, both of which need to drain the map to emit terminal updates.
func (a *Adapter) takeActiveMonitors(sessionID string) map[string]string {
	a.mu.Lock()
	defer a.mu.Unlock()
	monitors := a.activeMonitors[sessionID]
	delete(a.activeMonitors, sessionID)
	return monitors
}

// extractMonitorEvents parses every `<task-notification>` envelope out of an
// agent_message_chunk text, replaces each match with empty string, and
// returns the cleaned text plus the parsed events in order.
//
// Each event carries the matched taskID (so the caller can route to the right
// Monitor's toolCallID) and the event body (the actual line stdout produced).
func extractMonitorEvents(text string) (cleaned string, events []monitorEvent) {
	matches := monitorTaskNotifRE.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text, nil
	}
	events = make([]monitorEvent, 0, len(matches))
	var b strings.Builder
	prev := 0
	for _, m := range matches {
		// m: [matchStart, matchEnd, taskIdStart, taskIdEnd, eventStart, eventEnd]
		b.WriteString(text[prev:m[0]])
		events = append(events, monitorEvent{
			TaskID: text[m[2]:m[3]],
			Body:   text[m[4]:m[5]],
		})
		prev = m[1]
	}
	b.WriteString(text[prev:])
	return b.String(), events
}

// isMonitorHumanEcho returns true when text is a stripped, orphan "Human:"
// prefix the model emitted while trying to echo a task-notification envelope.
// Such chunks should be dropped to keep them out of the chat UI.
func isMonitorHumanEcho(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	return monitorHumanEchoRE.MatchString(trimmed)
}

// monitorEvent represents one Monitor stdout line decoded from a
// `<task-notification>` envelope.
type monitorEvent struct {
	TaskID string
	Body   string
}

// monitorPayloadCap bounds how many recent event lines we attach to the
// Monitor's NormalizedPayload. Keeping this small keeps message metadata
// reasonable for long-running Monitors (a CI watch can fire hundreds of
// events) while still giving the user a tail of activity in the UI.
const monitorPayloadCap = 10

// monitorPayloadView is the structured shape we tuck into the Generic tool
// payload's Output field for Monitor tools. It tells the frontend how to
// render the card (count, recent tail, command) without needing the
// adapter to invent a new NormalizedPayload kind.
type monitorPayloadView struct {
	Kind         string   `json:"kind"`
	TaskID       string   `json:"task_id"`
	Command      string   `json:"command,omitempty"`
	EventCount   int      `json:"event_count"`
	RecentEvents []string `json:"recent_events,omitempty"`
	Ended        bool     `json:"ended,omitempty"`
	EndReason    string   `json:"end_reason,omitempty"`
}

// seedMonitorView writes an empty Monitor view onto the Generic payload at
// registration time so the frontend can switch the card renderer immediately
// — before any events fire. Without this seed a CI watch with a long quiet
// startup would render as a generic tool_call for minutes.
func seedMonitorView(payload *streams.NormalizedPayload, taskID, command string) {
	if payload == nil {
		return
	}
	g := payload.Generic()
	if g == nil {
		return
	}
	view := readMonitorView(g)
	if view.TaskID == "" {
		view.TaskID = taskID
	}
	if view.Command == "" {
		view.Command = command
	}
	g.Output = monitorOutputWrapper(view)
}

// updateMonitorPayloadView mutates the Monitor tool's NormalizedPayload to
// reflect a new event (or a terminal state). Returns the same payload pointer
// so callers can pass it back into the synthetic AgentEvent.
//
// We piggyback on the Generic payload that was created by `normalizeGeneric`
// when the original tool_call (kind="other") arrived. The adapter's existing
// activeToolCalls map already holds it.
func appendMonitorEvent(payload *streams.NormalizedPayload, taskID, command, body string) *streams.NormalizedPayload {
	if payload == nil {
		return nil
	}
	g := payload.Generic()
	if g == nil {
		// If somebody upstream changed the tool_call kind we won't have a
		// Generic payload — leave the payload alone rather than reconstructing.
		return payload
	}
	view := readMonitorView(g)
	if view.TaskID == "" {
		view.TaskID = taskID
	}
	if view.Command == "" {
		view.Command = command
	}
	view.EventCount++
	view.RecentEvents = appendCapped(view.RecentEvents, body, monitorPayloadCap)
	g.Output = monitorOutputWrapper(view)
	return payload
}

// markMonitorEnded sets the terminal flag and reason on the Monitor view
// without bumping the event counter.
func markMonitorEnded(payload *streams.NormalizedPayload, reason string) *streams.NormalizedPayload {
	if payload == nil {
		return nil
	}
	g := payload.Generic()
	if g == nil {
		return payload
	}
	view := readMonitorView(g)
	view.Ended = true
	view.EndReason = reason
	g.Output = monitorOutputWrapper(view)
	return payload
}

// readMonitorView extracts the existing Monitor view from a Generic payload
// (the wrapper persists across tool_call_updates because the adapter mutates
// the same payload pointer). Returns a zero-valued view when no Monitor data
// is attached yet.
func readMonitorView(g *streams.GenericPayload) monitorPayloadView {
	if g == nil || g.Output == nil {
		return monitorPayloadView{Kind: monitorToolName}
	}
	wrapper, ok := g.Output.(map[string]any)
	if !ok {
		return monitorPayloadView{Kind: monitorToolName}
	}
	raw, ok := wrapper["monitor"].(map[string]any)
	if !ok {
		return monitorPayloadView{Kind: monitorToolName}
	}
	view := monitorPayloadView{Kind: monitorToolName}
	if s, ok := raw["task_id"].(string); ok {
		view.TaskID = s
	}
	if s, ok := raw["command"].(string); ok {
		view.Command = s
	}
	if n, ok := raw["event_count"].(float64); ok {
		view.EventCount = int(n)
	} else if n, ok := raw["event_count"].(int); ok {
		view.EventCount = n
	}
	if list, ok := raw["recent_events"].([]any); ok {
		view.RecentEvents = make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok {
				view.RecentEvents = append(view.RecentEvents, s)
			}
		}
	} else if list, ok := raw["recent_events"].([]string); ok {
		view.RecentEvents = append([]string{}, list...)
	}
	if b, ok := raw["ended"].(bool); ok {
		view.Ended = b
	}
	if s, ok := raw["end_reason"].(string); ok {
		view.EndReason = s
	}
	return view
}

// monitorOutputWrapper boxes the typed view in the `{monitor: {...}}` shape
// the frontend monitor card reads.
func monitorOutputWrapper(view monitorPayloadView) map[string]any {
	return map[string]any{"monitor": map[string]any{
		"kind":          view.Kind,
		"task_id":       view.TaskID,
		"command":       view.Command,
		"event_count":   view.EventCount,
		"recent_events": view.RecentEvents,
		"ended":         view.Ended,
		"end_reason":    view.EndReason,
	}}
}

// appendCapped appends s to xs, keeping at most cap most-recent entries.
func appendCapped(xs []string, s string, capacity int) []string {
	xs = append(xs, s)
	if len(xs) > capacity {
		return xs[len(xs)-capacity:]
	}
	return xs
}

// monitorEventEvent constructs a synthetic tool_call_update AgentEvent for a
// Monitor event. The frontend appends the body line to the Monitor's content
// stream and renders an updated event count.
func monitorEventEvent(sessionID, toolCallID string, body string, payload *streams.NormalizedPayload) AgentEvent {
	return AgentEvent{
		Type:              streams.EventTypeToolUpdate,
		SessionID:         sessionID,
		ToolCallID:        toolCallID,
		ToolStatus:        "in_progress",
		NormalizedPayload: payload,
		ToolCallContents: []streams.ToolCallContentItem{
			{Type: "content", Content: &streams.ContentBlock{Type: "text", Text: body}},
		},
	}
}

// captureReplayMonitor walks a single replayed session/update during
// session/load history replay and registers any Monitor it finds in
// `activeMonitors`. The post-replay sweep then knows which Monitors to mark
// ended-on-restart. This is purely an internal capture path — the replay
// notifications themselves are still suppressed from reaching clients.
//
// Two notification shapes can register a Monitor:
//   - `tool_call` with `_meta.claudeCode.toolName == "Monitor"` (records by toolCallID;
//     we don't yet know taskID until the registration update arrives)
//   - `tool_call_update` whose `rawOutput` matches the registration banner
//     (records the taskID -> toolCallID mapping)
//
// A `tool_call_update` whose status is terminal (`completed` with non-banner
// rawOutput, `failed`, `cancelled`) means the Monitor already ended in
// history — drop it from the map.
func (a *Adapter) captureReplayMonitor(sessionID string, u acp.SessionUpdate) {
	if tc := u.ToolCall; tc != nil {
		if _, ok := recognizeMonitorTaskCall(tc); ok {
			a.trackMonitor(sessionID, replayPendingTaskID(string(tc.ToolCallId)), string(tc.ToolCallId))
		}
	}
	if tcu := u.ToolCallUpdate; tcu != nil {
		toolCallID := string(tcu.ToolCallId)
		if taskID, ok := recognizeMonitorRegistration(tcu.Meta, tcu.RawOutput); ok {
			// Replace any pending-by-toolCallID placeholder with the real taskID.
			a.replaceMonitorPendingKey(sessionID, toolCallID, taskID)
			a.trackMonitor(sessionID, taskID, toolCallID)
			return
		}
		// Terminal update for a tracked Monitor — drop it from the map so the
		// post-replay sweep doesn't double-emit a cancellation.
		if tcu.Status != nil {
			s := string(*tcu.Status)
			if s == "completed" || s == "failed" || s == "cancelled" {
				a.dropMonitorByToolCallID(sessionID, toolCallID)
			}
		}
	}
}

// replayPendingTaskID synthesizes a placeholder taskID for a Monitor we saw
// during replay before the registration banner arrived. It's namespaced by
// toolCallID so it can never collide with a real taskID. Replaced in place
// once the banner reveals the real taskID via replaceMonitorPendingKey.
func replayPendingTaskID(toolCallID string) string {
	return "_pending_" + toolCallID
}

// replaceMonitorPendingKey rewrites an entry in activeMonitors that was
// registered during replay with a placeholder taskID once the real taskID
// arrives via the registration banner.
func (a *Adapter) replaceMonitorPendingKey(sessionID, toolCallID, realTaskID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	monitors := a.activeMonitors[sessionID]
	if monitors == nil {
		return
	}
	delete(monitors, replayPendingTaskID(toolCallID))
	monitors[realTaskID] = toolCallID
}

// dropMonitorByToolCallID removes any entry in activeMonitors mapped to the
// given toolCallID. Used during replay to discard Monitors that already had
// a terminal status in history.
func (a *Adapter) dropMonitorByToolCallID(sessionID, toolCallID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	monitors := a.activeMonitors[sessionID]
	if monitors == nil {
		return
	}
	for taskID, tcID := range monitors {
		if tcID == toolCallID {
			delete(monitors, taskID)
		}
	}
	if len(monitors) == 0 {
		delete(a.activeMonitors, sessionID)
	}
}

// sweepMonitorsOnPromptEnd emits a terminal `complete` tool_call_update for
// every tracked Monitor in a session and clears the map. Called when the
// parent prompt naturally completes — the Monitor process exits with the
// agent turn, so the card should flip from "watching" to "ended".
func (a *Adapter) sweepMonitorsOnPromptEnd(sessionID string) {
	for _, toolCallID := range a.takeActiveMonitors(sessionID) {
		a.mu.Lock()
		payload := a.activeToolCalls[toolCallID]
		markMonitorEnded(payload, "exited")
		delete(a.activeToolCalls, toolCallID)
		a.mu.Unlock()
		a.sendUpdate(monitorTerminalEvent(sessionID, toolCallID, toolStatusComplete, "Monitor exited", payload))
	}
}

// sweepMonitorsOnReplayEnd emits a `cancelled` tool_call_update for every
// Monitor reconstructed from session-load history. The agent process restart
// killed the underlying script, so any Monitor that was in-flight before the
// restart is dead — we surface that to the UI rather than leaving the card
// stuck in "watching".
func (a *Adapter) sweepMonitorsOnReplayEnd(sessionID string) {
	for _, toolCallID := range a.takeActiveMonitors(sessionID) {
		a.mu.Lock()
		payload := a.activeToolCalls[toolCallID]
		markMonitorEnded(payload, "session_restart")
		delete(a.activeToolCalls, toolCallID)
		a.mu.Unlock()
		a.sendUpdate(monitorTerminalEvent(sessionID, toolCallID, "cancelled", "Monitor ended (session restart)", payload))
	}
}

// monitorTerminalEvent constructs the synthetic tool_call_update emitted when
// a tracked Monitor must be marked finished — either because the parent
// prompt ended (status "complete") or because the agent process restarted
// (status "cancelled" with a "session restart" note in the contents).
func monitorTerminalEvent(sessionID, toolCallID, status, note string, payload *streams.NormalizedPayload) AgentEvent {
	var contents []streams.ToolCallContentItem
	if note != "" {
		contents = []streams.ToolCallContentItem{
			{Type: "content", Content: &streams.ContentBlock{Type: "text", Text: note}},
		}
	}
	return AgentEvent{
		Type:              streams.EventTypeToolUpdate,
		SessionID:         sessionID,
		ToolCallID:        toolCallID,
		ToolStatus:        status,
		NormalizedPayload: payload,
		ToolCallContents:  contents,
	}
}
