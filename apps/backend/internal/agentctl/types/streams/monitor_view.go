package streams

// MonitorSubkind is the `kind` value the ACP adapter stamps on the structured
// Monitor view it tucks into a Generic tool payload's Output (see
// server/adapter/transport/acp/monitor.go). It is the shared contract between
// the adapter (the producer) and consumers such as the orchestrator's
// background-work classifier, so neither side has to string-match a tool name.
const MonitorSubkind = "Monitor"

// Monitor view map keys — the shape acp/monitor.go writes as
//
//	Generic.Output = {"monitor": {"kind": …, "task_id": …, "ended": …, …}}
//
// Exported so the producer (acp/monitor.go's monitorOutputWrapper and
// readMonitorView) builds and reads the map from these very constants rather
// than re-typing the literals on its side of the package boundary. That is the
// point: while the two sides each spelled the strings out, renaming a key in the
// producer still compiled and silently reverted Monitor sessions to the coarse
// busy signal. acp's monitor_contract_test.go pins the producer→consumer round
// trip so the contract can't drift unnoticed again.
const (
	MonitorViewKey             = "monitor"
	MonitorViewKindKey         = "kind"
	MonitorViewTaskIDKey       = "task_id"
	MonitorViewCommandKey      = "command"
	MonitorViewEventCountKey   = "event_count"
	MonitorViewRecentEventsKey = "recent_events"
	MonitorViewEndedKey        = "ended"
	MonitorViewEndReasonKey    = "end_reason"
)

// IsActiveMonitor reports whether this payload is a live Claude Monitor watch:
// a Generic payload carrying the structured Monitor view whose `ended` flag is
// not set. claude-agent-acp tags Monitor with `_meta.claudeCode.toolName:
// "Monitor"` and `kind:"other"`, so it normalizes to a Generic payload rather
// than a dedicated kind (see acp/monitor.go). A Monitor is long-running
// background work the foreground turn is not actively generating against, so an
// active one is treated like any other spawned background task by the busy
// signal.
//
// Provenance: a Generic payload's Output is otherwise the agent's *own* raw tool
// result — normalize.go's NormalizeToolResult assigns it verbatim — so this
// predicate must not fire for an unrelated tool that merely happens to serialize
// a `monitor` key. It therefore demands the full shape the adapter writes,
// including a non-empty `task_id`: that field is only ever populated once the
// Monitor registration banner yields a real task ID (seedMonitorView), so a view
// without one did not come off the Monitor path. Note the Generic payload's
// `Name` is NOT a usable discriminator here — it carries the ACP tool *kind*,
// which is "other" for Monitor, not "Monitor".
//
// Returns false for a nil payload, a non-Generic payload, a Generic payload with
// no Monitor view, a view carrying no task ID, or a Monitor that has ended.
func (p *NormalizedPayload) IsActiveMonitor() bool {
	if p == nil || p.kind != ToolKindGeneric || p.generic == nil {
		return false
	}
	wrapper, ok := p.generic.Output.(map[string]any)
	if !ok {
		return false
	}
	view, ok := wrapper[MonitorViewKey].(map[string]any)
	if !ok {
		return false
	}
	if kind, _ := view[MonitorViewKindKey].(string); kind != MonitorSubkind {
		return false
	}
	if taskID, _ := view[MonitorViewTaskIDKey].(string); taskID == "" {
		return false
	}
	ended, _ := view[MonitorViewEndedKey].(bool)
	return !ended
}
