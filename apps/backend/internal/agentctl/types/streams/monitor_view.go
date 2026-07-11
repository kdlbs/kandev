package streams

// MonitorSubkind is the `kind` value the ACP adapter stamps on the structured
// Monitor view it tucks into a Generic tool payload's Output (see
// server/adapter/transport/acp/monitor.go). It is the shared contract between
// the adapter (the producer) and consumers such as the orchestrator's
// background-work classifier, so neither side has to string-match a tool name.
const MonitorSubkind = "Monitor"

// Monitor view map keys — the shape acp/monitor.go writes as
// Generic.Output = {"monitor": {"kind": ..., "ended": ...}}. Kept here next to
// the predicate that reads them so producer and consumer stay in lockstep.
const (
	monitorViewKey      = "monitor"
	monitorViewKindKey  = "kind"
	monitorViewEndedKey = "ended"
)

// IsActiveMonitor reports whether this payload is a live Claude Monitor watch:
// a Generic payload carrying the structured Monitor view whose `ended` flag is
// not set. claude-agent-acp tags Monitor with `_meta.claudeCode.toolName:
// "Monitor"` and `kind:"other"`, so it normalizes to a Generic payload rather
// than a dedicated kind (see acp/monitor.go). A Monitor is long-running
// background work the foreground turn is not actively generating against, so an
// active one is treated like any other spawned background task by the busy
// signal. Returns false for a nil payload, a non-Generic payload, a Generic
// payload with no Monitor view, or a Monitor that has already ended.
func (p *NormalizedPayload) IsActiveMonitor() bool {
	if p == nil || p.kind != ToolKindGeneric || p.generic == nil {
		return false
	}
	wrapper, ok := p.generic.Output.(map[string]any)
	if !ok {
		return false
	}
	view, ok := wrapper[monitorViewKey].(map[string]any)
	if !ok {
		return false
	}
	if kind, _ := view[monitorViewKindKey].(string); kind != MonitorSubkind {
		return false
	}
	ended, _ := view[monitorViewEndedKey].(bool)
	return !ended
}
