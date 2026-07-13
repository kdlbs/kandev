package streams

// MonitorSubkind is the `kind` value the ACP adapter stamps on the structured
// Monitor view it tucks into a Generic tool payload's Output (see
// server/adapter/transport/acp/monitor.go).
const MonitorSubkind = "Monitor"

// Monitor view map keys — the shape acp/monitor.go writes as
//
//	Generic.Output = {"monitor": {"kind": …, "task_id": …, "ended": …, …}}
//
// This map is the *presentation* contract: it is what the frontend Monitor card
// renders from. It is NOT what the background-work classifier reads — see
// MonitorPayload and IsActiveMonitor below for why.
//
// Exported so acp/monitor.go builds and reads the map from these very constants
// instead of re-typing the literals on its side of the package boundary; while
// each side spelled the strings out independently, renaming a key on one side
// still compiled and silently broke the other.
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

// MonitorPayload is adapter-attested Monitor identity: proof that the ACP adapter
// itself recognized this tool call as a Claude Monitor watch, carried as a typed
// sibling of GenericPayload rather than as a key inside GenericPayload.Output.
//
// The distinction is the whole point. `Generic.Output` is assigned the agent's
// raw tool result verbatim (NormalizeToolResult), so any structure *inside* it is
// agent-shaped data — an unrelated tool whose result happened to serialize a
// monitor-looking map would be indistinguishable from a real Monitor, no matter
// how many fields the classifier demanded. This slot, by contrast, is only ever
// written by the adapter's Monitor recognizer (via SetMonitorIdentity), which
// gates on ACP `_meta.claudeCode.toolName` — metadata the claude-agent-acp
// wrapper sets, which model tool output cannot reach. Nothing in the normalize
// path ever copies agent data here, so a payload carrying it provably came off
// the Monitor path.
//
// It survives the agentctl→orchestrator boundary as its own `monitor` key on the
// serialized payload, so the classifier on the far side reads the same attestation.
type MonitorPayload struct {
	TaskID string `json:"task_id"`
	Ended  bool   `json:"ended,omitempty"`
}

// Monitor returns the adapter-attested Monitor identity, or nil when this payload
// was not recognized as a Monitor.
func (p *NormalizedPayload) Monitor() *MonitorPayload {
	if p == nil {
		return nil
	}
	return p.monitor
}

// SetMonitorIdentity records that the ACP adapter recognized this tool call as a
// Monitor watch, and its current terminal state. Only the adapter's Monitor
// recognizer may call this — it is the attestation IsActiveMonitor trusts.
func (p *NormalizedPayload) SetMonitorIdentity(taskID string, ended bool) {
	if p == nil {
		return
	}
	if p.monitor == nil {
		p.monitor = &MonitorPayload{}
	}
	if taskID != "" {
		p.monitor.TaskID = taskID
	}
	p.monitor.Ended = ended
}

// IsActiveMonitor reports whether this payload is a live Claude Monitor watch.
// claude-agent-acp tags Monitor with `_meta.claudeCode.toolName: "Monitor"` and
// `kind:"other"`, so it normalizes to a Generic payload rather than a dedicated
// kind (see acp/monitor.go). A Monitor is long-running background work the
// foreground turn is not actively generating against, so an active one is treated
// like any other spawned background task by the busy signal (ADR-0035).
//
// It classifies on the adapter's attestation (MonitorPayload), never on the shape
// of Generic.Output — that field is the agent's own raw tool result and so can
// neither prove nor disprove provenance. This is what keeps the ADR-0035 contract
// honest: an agent we don't recognize cannot relax its own busy gate by emitting a
// monitor-shaped tool result, and keeps the historical reject-while-RUNNING
// behavior. (The payload's `Name` is likewise unusable as a discriminator: it
// carries the ACP tool kind, which is "other" for Monitor, not "Monitor".)
//
// Returns false for a nil payload, a payload the adapter never recognized as a
// Monitor, or a Monitor that has already ended.
func (p *NormalizedPayload) IsActiveMonitor() bool {
	if p == nil || p.monitor == nil {
		return false
	}
	return !p.monitor.Ended
}
