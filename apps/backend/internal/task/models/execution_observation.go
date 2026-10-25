package models

import contract "github.com/kandev/kandev/internal/common/sandbox"

// ExecutionObservation is ordered independently of task lifecycle state.
type ExecutionObservation struct {
	SchemaVersion      int    `json:"schema_version"`
	ObserverGeneration uint64 `json:"observer_generation"`
	Revision           uint64 `json:"revision"`
	SessionID          string `json:"session_id"`
	TaskID             string `json:"task_id"`
	SnapshotSequence   uint64 `json:"snapshot_sequence"`
	contract.Observation
}

func (o *ExecutionObservation) ValidFields() bool {
	return o != nil && o.SchemaVersion == 1 && o.ObserverGeneration > 0 && o.Revision > 0 && o.SessionID != "" && o.TaskID != "" && o.SnapshotSequence > 0 && o.Observation.ValidFields()
}
