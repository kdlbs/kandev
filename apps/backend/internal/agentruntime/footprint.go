package agentruntime

import "time"

// SessionRuntimeFootprint is one session's authoritative footprint row. It
// carries counts, bytes, runtime state, and timestamps only, never transcript
// content, a credential, a resume token, or a command line.
type SessionRuntimeFootprint struct {
	SessionID           string
	TaskID              string
	ExecutionID         string
	Runtime             string
	Status              string
	Processes           int
	CommittedBytes      uint64
	ResidentBytes       uint64
	UnreadableProcesses int
	LastActivityAt      time.Time
}

// RuntimeFootprintSnapshot is one observation of what the live agent runtimes
// cost. The counters are installation totals; Runtimes is the bounded
// per-session projection that attributes those totals to the sessions holding
// them.
//
// Complete is false when the observation failed or could not be taken. A caller
// must never read an incomplete snapshot's totals as current.
type RuntimeFootprintSnapshot struct {
	LiveRuntimes       int
	ProcessCount       int
	CommittedBytes     uint64
	ResidentBytes      uint64
	UnreadableRuntimes int
	Runtimes           []SessionRuntimeFootprint
	Complete           bool
}
