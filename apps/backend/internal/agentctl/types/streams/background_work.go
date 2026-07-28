package streams

// BackgroundWorkKind identifies a workload whose lifecycle was recognized by
// an adapter. It is deliberately separate from ToolKind: ToolKind is shared
// presentation data, while this value is adapter-issued provenance used for
// prompt admission and runtime accounting.
type BackgroundWorkKind string

const (
	BackgroundWorkKindSubagent BackgroundWorkKind = "subagent"
	BackgroundWorkKindShell    BackgroundWorkKind = "shell"
	BackgroundWorkKindMonitor  BackgroundWorkKind = "monitor"
)

// BackgroundWorkPayload is typed adapter attestation that Kandev recognizes
// this workload's launch and terminal lifecycle.
type BackgroundWorkPayload struct {
	Kind     BackgroundWorkKind `json:"kind"`
	WorkID   string             `json:"work_id,omitempty"`
	Detached bool               `json:"detached,omitempty"`
	Ended    bool               `json:"ended,omitempty"`
}

// BackgroundWork returns adapter-issued background-work provenance, or nil
// when the normalized payload is presentation-only.
func (p *NormalizedPayload) BackgroundWork() *BackgroundWorkPayload {
	if p == nil {
		return nil
	}
	return p.backgroundWork
}

// SetBackgroundWorkIdentity records adapter-issued lifecycle provenance.
// Adapter recognizers are the only production callers.
func (p *NormalizedPayload) SetBackgroundWorkIdentity(
	kind BackgroundWorkKind,
	workID string,
	detached bool,
	ended bool,
) {
	if p == nil || kind == "" {
		return
	}
	if p.backgroundWork == nil {
		p.backgroundWork = &BackgroundWorkPayload{}
	}
	p.backgroundWork.Kind = kind
	if workID != "" {
		p.backgroundWork.WorkID = workID
	}
	p.backgroundWork.Detached = detached
	p.backgroundWork.Ended = ended
}

// IsActiveBackgroundWork reports whether an adapter attested a live workload.
func (p *NormalizedPayload) IsActiveBackgroundWork() bool {
	return p != nil && p.backgroundWork != nil && !p.backgroundWork.Ended
}

// IsDetachedBackgroundLaunch distinguishes a terminal launch-tool result from
// terminal evidence for the launched workload itself.
func (p *NormalizedPayload) IsDetachedBackgroundLaunch() bool {
	return p.IsActiveBackgroundWork() && p.backgroundWork.Detached
}
