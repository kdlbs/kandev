package streams

import (
	"time"
)

// WorkloadKind identifies the normalized category of a background workload.
type WorkloadKind string

const (
	WorkloadKindShell    WorkloadKind = "shell"
	WorkloadKindSubagent WorkloadKind = "subagent"
	WorkloadKindMonitor  WorkloadKind = "monitor"
	WorkloadKindCustom   WorkloadKind = "custom"
	WorkloadKindUnknown  WorkloadKind = "unknown"
)

// NormalizeWorkloadKind maps arbitrary provider strings to a known WorkloadKind.
func NormalizeWorkloadKind(k string) WorkloadKind {
	switch WorkloadKind(k) {
	case WorkloadKindShell, WorkloadKindSubagent, WorkloadKindMonitor, WorkloadKindCustom:
		return WorkloadKind(k)
	default:
		return WorkloadKindUnknown
	}
}

// RunState represents the lifecycle status of a background workload execution.
type RunState string

const (
	RunStateRunning     RunState = "running"
	RunStateWaiting     RunState = "waiting"
	RunStateCompleted   RunState = "completed"
	RunStateFailed      RunState = "failed"
	RunStateInterrupted RunState = "interrupted"
	RunStateEnded       RunState = "ended"
	RunStateUnknown     RunState = "unknown"
)

// IsTerminal reports whether the RunState is final.
func (s RunState) IsTerminal() bool {
	switch s {
	case RunStateCompleted, RunStateFailed, RunStateInterrupted, RunStateEnded:
		return true
	default:
		return false
	}
}

// WorkloadActionKind identifies an action that can be executed on a workload.
type WorkloadActionKind string

const (
	WorkloadActionStop       WorkloadActionKind = "stop"
	WorkloadActionInterrupt  WorkloadActionKind = "interrupt"
	WorkloadActionWriteInput WorkloadActionKind = "write_input"
	WorkloadActionCloseInput WorkloadActionKind = "close_input"
)

// ActionAvailabilityReason explains why an action cannot currently be executed.
type ActionAvailabilityReason string

const (
	ActionReasonUnsupported      ActionAvailabilityReason = "unsupported"
	ActionReasonDisconnected     ActionAvailabilityReason = "disconnected"
	ActionReasonNotRunning       ActionAvailabilityReason = "not_running"
	ActionReasonOwnershipUnknown ActionAvailabilityReason = "ownership_unknown"
	ActionReasonOperationPending ActionAvailabilityReason = "operation_pending"
)

// ActionCapability expresses whether an action is supported and available.
type ActionCapability struct {
	Supported bool   `json:"supported"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// WorkloadCapabilities defines the observable data and action operations supported by a provider.
type WorkloadCapabilities struct {
	Discovery         string                                  `json:"discovery"` // "snapshot" | "events_only"
	Output            string                                  `json:"output"`    // "stream" | "snapshot" | "none"
	Transcript        bool                                    `json:"transcript"`
	Parentage         bool                                    `json:"parentage"`
	ReasoningSummary  bool                                    `json:"reasoning_summary"`
	AttributableUsage bool                                    `json:"attributable_usage"`
	Actions           map[WorkloadActionKind]ActionCapability `json:"actions,omitempty"`
}

// WorkloadRunObservation captures a normalized observation of a background workload and run.
type WorkloadRunObservation struct {
	SessionID       string               `json:"session_id,omitempty"`
	WorkID          string               `json:"work_id"`
	RunID           string               `json:"run_id,omitempty"`
	Kind            WorkloadKind         `json:"kind"`
	Title           string               `json:"title"`
	State           RunState             `json:"state"`
	ParentWorkID    string               `json:"parent_work_id,omitempty"`
	OriginTurnID    string               `json:"origin_turn_id,omitempty"`
	SourceMessageID string               `json:"source_message_id,omitempty"`
	SourceCallID    string               `json:"source_call_id,omitempty"`
	ExitCode        *int                 `json:"exit_code,omitempty"`
	Capabilities    WorkloadCapabilities `json:"capabilities"`
	Output          string               `json:"output,omitempty"`
	OutputTruncated bool                 `json:"output_truncated,omitempty"`
	OutputOffset    int64                `json:"output_offset,omitempty"`
	StartedAt       *time.Time           `json:"started_at,omitempty"`
	FinishedAt      *time.Time           `json:"finished_at,omitempty"`
	Revision        int64                `json:"revision"`
}

func (o WorkloadRunObservation) GetSessionID() string {
	return o.SessionID
}

// BackgroundWorkSnapshot captures the full collection of workloads for a session at a moment in time.
type BackgroundWorkSnapshot struct {
	Workloads  []WorkloadRunObservation `json:"workloads"`
	CapturedAt time.Time                `json:"captured_at"`
}

// BackgroundWorkActionRequest is a request to perform an action on a workload.
type BackgroundWorkActionRequest struct {
	WorkID      string             `json:"work_id"`
	RunID       string             `json:"run_id,omitempty"`
	Action      WorkloadActionKind `json:"action"`
	Data        string             `json:"data,omitempty"`
	OperationID string             `json:"operation_id,omitempty"`
}

// BackgroundWorkActionResponse is the result of performing an action on a workload.
type BackgroundWorkActionResponse struct {
	Success   bool               `json:"success"`
	WorkID    string             `json:"work_id"`
	RunID     string             `json:"run_id,omitempty"`
	Action    WorkloadActionKind `json:"action"`
	Error     string             `json:"error,omitempty"`
	Uncertain bool               `json:"uncertain,omitempty"`
}

// WorkloadOutputChunk carries an incremental log or output stream chunk for a workload.
type WorkloadOutputChunk struct {
	SessionID string `json:"session_id,omitempty"`
	WorkID    string `json:"work_id"`
	RunID     string `json:"run_id,omitempty"`
	Chunk     string `json:"chunk"`
	Offset    int64  `json:"offset"`
	Truncated bool   `json:"truncated,omitempty"`
	Stream    string `json:"stream,omitempty"` // "stdout" | "stderr"
}

func (c WorkloadOutputChunk) GetSessionID() string {
	return c.SessionID
}
