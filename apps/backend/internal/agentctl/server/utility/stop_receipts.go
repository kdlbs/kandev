package utility

import "time"

const ManagedStopCoverage = "current_manager_and_recorded_attempts"

// ManagedProcessStopReceipt describes a completed current-manager teardown.
// It never certifies an unobserved predecessor or a remote provider request.
type ManagedProcessStopReceipt struct {
	Version                  int       `json:"version"`
	InstanceID               string    `json:"instance_id"`
	OwnerID                  string    `json:"owner_id"`
	ObservedAt               time.Time `json:"observed_at"`
	AdmissionClosed          bool      `json:"admission_closed"`
	OperationsDrained        bool      `json:"operations_drained"`
	ManagedProcessesStopped  bool      `json:"managed_processes_stopped"`
	InferenceCensusKnown     bool      `json:"inference_census_known"`
	RecordedChildrenTerminal bool      `json:"recorded_children_terminal"`
	Coverage                 string    `json:"coverage"`
}

// InferenceStopSettlementReport is limited to managed bindings for one instance.
// A successful local claim settlement is not a remote cancellation certificate.
type InferenceStopSettlementReport struct {
	Scope                          string `json:"scope"`
	Status                         string `json:"status"`
	ManagedClaims                  int    `json:"managed_claims"`
	MatchedTerminal                int    `json:"matched_terminal"`
	Settled                        int    `json:"settled"`
	Unresolved                     int    `json:"unresolved"`
	UnmatchedRetained              int    `json:"unmatched_retained"`
	JournalAcknowledgementObserved bool   `json:"journal_acknowledgement_observed"`
}

// InferenceStopObservation contains only current-instance results, not history.
type InferenceStopObservation struct {
	InstanceID    string                         `json:"instance_id"`
	SnapshotKnown bool                           `json:"snapshot_known"`
	ManagedStop   *ManagedProcessStopReceipt     `json:"managed_process_stop,omitempty"`
	Settlement    *InferenceStopSettlementReport `json:"settlement,omitempty"`
	Evidence      string                         `json:"evidence"`
}

// RecordedAttemptsTerminal checks recorded children only; it cannot fill gaps in
// historical coverage when the snapshot census is unknown.
func RecordedAttemptsTerminal(snapshot *AttemptSnapshot) bool {
	if snapshot == nil {
		return false
	}
	for _, child := range snapshot.Attempts {
		if !child.Known || !child.Settled || (child.State != AttemptTerminated && child.State != AttemptNotStarted) {
			return false
		}
	}
	return true
}
