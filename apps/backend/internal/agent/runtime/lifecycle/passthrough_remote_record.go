package lifecycle

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/agentctl/types"
)

const metadataKeySSHPassthroughState = "ssh_passthrough_state"
const (
	remotePromptPending   = "pending"
	remotePromptUncertain = "uncertain"
	remotePromptWritten   = "written"
)

type remotePassthroughRecord struct {
	types.PassthroughTarget
	InstanceID                string   `json:"instance_id"`
	StartOperationID          string   `json:"start_operation_id"`
	EventSequence             uint64   `json:"event_sequence"`
	PreparedGeneration        uint64   `json:"prepared_generation"`
	InitialSubmissionSequence uint64   `json:"initial_submission_sequence,omitempty"`
	InitialPromptState        string   `json:"initial_prompt_state,omitempty"`
	InitialSubmissionIDs      []string `json:"initial_submission_ids,omitempty"`
	InitialExpectedChunks     int      `json:"initial_expected_chunks,omitempty"`
}

func remotePassthroughRecordFor(execution *AgentExecution) remotePassthroughRecord {
	value, _ := execution.metadataValue(metadataKeySSHPassthroughState)
	data, err := json.Marshal(value)
	if err != nil {
		return remotePassthroughRecord{}
	}
	var record remotePassthroughRecord
	_ = json.Unmarshal(data, &record)
	return record
}

func (m *Manager) saveRemotePassthroughRecord(ctx context.Context, execution *AgentExecution, record remotePassthroughRecord) error {
	execution.setMetadataValue(metadataKeySSHPassthroughState, record)
	return m.persistExecutorRunningResult(ctx, execution)
}

func (m *Manager) updateRemotePassthroughRecord(ctx context.Context, execution *AgentExecution, update func(*remotePassthroughRecord)) error {
	execution.remotePassthroughRecordMu.Lock()
	defer execution.remotePassthroughRecordMu.Unlock()
	record := remotePassthroughRecordFor(execution)
	update(&record)
	return m.saveRemotePassthroughRecord(ctx, execution, record)
}

func (m *Manager) advanceRemotePassthroughCursor(ctx context.Context, execution *AgentExecution, target types.PassthroughTarget, instanceID string, sequence uint64) (bool, error) {
	// Runtime teardown and cursor persistence share this outer guard. No client
	// source guard is held while the database or client endpoint is read.
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	return m.advanceRemotePassthroughCursorLocked(ctx, execution, target, instanceID, sequence)
}

// The caller holds remoteInstanceLifecycleMu through the complete save.
func (m *Manager) advanceRemotePassthroughCursorLocked(ctx context.Context, execution *AgentExecution, target types.PassthroughTarget, instanceID string, sequence uint64) (bool, error) {
	current, exists := m.executionStore.GetBySessionID(execution.SessionID)
	if !exists || current != execution || !m.passthroughProcessMatches(execution, target.ProcessID) {
		return false, nil
	}
	execution.remotePassthroughRecordMu.Lock()
	defer execution.remotePassthroughRecordMu.Unlock()
	previous := remotePassthroughRecordFor(execution)
	if previous.PassthroughTarget != target || previous.InstanceID != instanceID || sequence < previous.EventSequence {
		return false, nil
	}
	next := previous
	next.EventSequence = sequence
	if err := m.saveRemotePassthroughRecord(ctx, execution, next); err != nil {
		execution.setMetadataValue(metadataKeySSHPassthroughState, previous)
		return false, err
	}
	return true, nil
}

func (r *remotePassthroughRunner) recordStart(ctx context.Context, snapshot *types.PassthroughSnapshot, operationID string, initialChunks int) error {
	update := func(record *remotePassthroughRecord) {
		record.InstanceID = snapshot.InstanceID
		record.StartOperationID = operationID
		record.PassthroughTarget = snapshot.PassthroughTarget
		if snapshot.OperationID != operationID {
			record.ProcessID = ""
			record.Generation = snapshot.Generation + 1
			record.EventSequence = snapshot.Sequence
			record.InitialPromptState = ""
			record.InitialExpectedChunks = initialChunks
			if initialChunks > 0 {
				record.InitialPromptState = remotePromptPending
			}
			record.InitialSubmissionIDs = nil
			record.InitialSubmissionSequence = 0
		}
	}
	if r.manager != nil {
		return r.manager.updateRemotePassthroughRecord(ctx, r.execution, update)
	}
	r.execution.remotePassthroughRecordMu.Lock()
	defer r.execution.remotePassthroughRecordMu.Unlock()
	record := remotePassthroughRecordFor(r.execution)
	update(&record)
	r.execution.setMetadataValue(metadataKeySSHPassthroughState, record)
	return nil
}
