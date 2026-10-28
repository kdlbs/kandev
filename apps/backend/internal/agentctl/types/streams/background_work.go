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

// InspectionOnlyWorkloadCapabilities is the declaration for a workload an
// adapter can list but cannot stream or control. Producers should publish it
// explicitly so the read path does not have to infer a surface.
func InspectionOnlyWorkloadCapabilities() WorkloadCapabilities {
	return WorkloadCapabilities{
		Discovery: "events_only",
		Output:    "none",
	}
}

// BackgroundWorkPayload is typed adapter attestation that Kandev recognizes
// this workload's launch and terminal lifecycle.
type BackgroundWorkPayload struct {
	Kind     BackgroundWorkKind `json:"kind"`
	WorkID   string             `json:"work_id,omitempty"`
	Detached bool               `json:"detached,omitempty"`
	Ended    bool               `json:"ended,omitempty"`
	// RunID is the provider's identity for this execution of the workload when
	// the provider exposes one. It is never derived from WorkID: native Codex
	// uses an item ID for the workload and a process ID for the run.
	RunID string `json:"run_id,omitempty"`
	// Unverified marks an identified launch whose provider cannot deliver later
	// liveness or completion on the current connection. It is a negative marker
	// so an older peer that omits the field keeps the legacy verified meaning.
	Unverified bool `json:"unverified,omitempty"`
	// Capabilities is the adapter's verified declaration of what this workload
	// supports. It is nil for providers that publish no declaration, which
	// keeps the existing presentation-only behavior.
	Capabilities *WorkloadCapabilities `json:"capabilities,omitempty"`
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
//
// The workload is treated as verified unless a later call marks it unverified:
// an older peer that omits the negative marker keeps the legacy meaning. A
// provider whose delivery ends with the launch must call
// SetUnverifiedBackgroundWorkIdentity.
func (p *NormalizedPayload) SetBackgroundWorkIdentity(
	kind BackgroundWorkKind,
	workID string,
	detached bool,
	ended bool,
) {
	p.setBackgroundWorkIdentity(kind, workID, detached, ended, false)
}

// SetUnverifiedBackgroundWorkIdentity records an identified launch whose
// provider cannot deliver later liveness or completion on this connection.
// Inspection stays available; background activity must not claim it is live.
func (p *NormalizedPayload) SetUnverifiedBackgroundWorkIdentity(
	kind BackgroundWorkKind,
	workID string,
	detached bool,
	ended bool,
) {
	p.setBackgroundWorkIdentity(kind, workID, detached, ended, true)
}

func (p *NormalizedPayload) setBackgroundWorkIdentity(
	kind BackgroundWorkKind,
	workID string,
	detached bool,
	ended bool,
	unverified bool,
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
	p.backgroundWork.Unverified = unverified
}

// SetBackgroundWorkRunID records the provider's execution identity for this
// workload. Callers must pass the provider's own run key; an empty value
// leaves run identity unavailable rather than deriving it from WorkID.
func (p *NormalizedPayload) SetBackgroundWorkRunID(runID string) {
	if p == nil || p.backgroundWork == nil || runID == "" {
		return
	}
	p.backgroundWork.RunID = runID
}

// SetBackgroundWorkCapabilities records the adapter's verified capability
// declaration for this workload.
func (p *NormalizedPayload) SetBackgroundWorkCapabilities(capabilities WorkloadCapabilities) {
	if p == nil || p.backgroundWork == nil {
		return
	}
	copied := capabilities
	p.backgroundWork.Capabilities = &copied
}

// IsActiveBackgroundWork reports whether an adapter attested a live workload.
func (p *NormalizedPayload) IsActiveBackgroundWork() bool {
	return p != nil && p.backgroundWork != nil && !p.backgroundWork.Ended
}

// IsVerifiedBackgroundWork reports whether the adapter can confirm this
// workload's later liveness and completion on the current connection. An older
// peer that predates the negative marker is treated as verified, preserving
// the legacy parked-projection behavior.
func (p *NormalizedPayload) IsVerifiedBackgroundWork() bool {
	return p != nil && p.backgroundWork != nil && !p.backgroundWork.Unverified
}

// IsDetachedBackgroundLaunch distinguishes a terminal launch-tool result from
// terminal evidence for the launched workload itself.
func (p *NormalizedPayload) IsDetachedBackgroundLaunch() bool {
	return p.IsActiveBackgroundWork() && p.backgroundWork.Detached
}
