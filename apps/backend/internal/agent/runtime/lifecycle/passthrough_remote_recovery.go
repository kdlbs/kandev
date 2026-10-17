package lifecycle

import (
	"context"
	"errors"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (m *Manager) captureRemotePassthroughEvidence(ctx context.Context, instance *ExecutorInstance) error {
	if instance.Client == nil {
		return errors.New("remote passthrough controller is unavailable")
	}
	expectedID, err := recordedAgentctlInstanceID(instance.RuntimeName, instance.InstanceID, instance.Metadata)
	if err != nil {
		return err
	}
	snapshot, err := instance.Client.GetPassthrough(ctx)
	if err != nil {
		return err
	}
	if snapshot.InstanceID != expectedID || snapshot.SessionID != instance.SessionID {
		return agentctl.ErrPassthroughConflict
	}
	if snapshot.ProcessID == "" || snapshot.Status != types.ProcessStatusRunning {
		return errors.New("remote passthrough process is absent. Use Resume to restore the conversation")
	}
	instance.remotePassthroughSnapshot = snapshot
	execution := &AgentExecution{ID: instance.InstanceID, SessionID: instance.SessionID, metadata: instance.Metadata}
	record, err := reconcileRemoteInitialPrompt(ctx, instance.Client, *snapshot, remotePassthroughRecordFor(execution))
	if err != nil {
		return err
	}
	instance.Metadata[metadataKeySSHPassthroughState] = record
	return nil
}

var errRemoteInitialPromptUncertain = errors.New("initial prompt delivery is uncertain. Check the remote CLI before starting a fresh session")

func reconcileRemoteInitialPrompt(ctx context.Context, client *agentctl.Client, snapshot types.PassthroughSnapshot, record remotePassthroughRecord) (remotePassthroughRecord, error) {
	if record.InstanceID != snapshot.InstanceID || record.StartOperationID != snapshot.OperationID || record.Generation != snapshot.Generation || (record.ProcessID != "" && record.ProcessID != snapshot.ProcessID) {
		return record, agentctl.ErrPassthroughConflict
	}
	record.PassthroughTarget = snapshot.PassthroughTarget
	if record.InitialPromptState != remotePromptPending && record.InitialPromptState != remotePromptUncertain {
		return record, nil
	}
	if record.InitialExpectedChunks <= 0 || len(record.InitialSubmissionIDs) != record.InitialExpectedChunks {
		return record, errRemoteInitialPromptUncertain
	}
	for _, id := range record.InitialSubmissionIDs {
		receipt, err := client.PassthroughInputReceipt(ctx, snapshot.PassthroughTarget, id)
		if err != nil || receipt.Outcome != remotePromptWritten {
			return record, errRemoteInitialPromptUncertain
		}
		if receipt.Sequence > record.InitialSubmissionSequence {
			record.InitialSubmissionSequence = receipt.Sequence
		}
	}
	record.InitialPromptState = remotePromptWritten
	record.EventSequence = snapshot.Sequence
	return record, nil
}

func (m *Manager) reconcileRemotePrompt(ctx context.Context, execution *AgentExecution, snapshot types.PassthroughSnapshot) error {
	client, release, err := (&remotePassthroughRunner{execution: execution}).client()
	if err != nil {
		return err
	}
	defer release()
	execution.remotePassthroughRecordMu.Lock()
	defer execution.remotePassthroughRecordMu.Unlock()
	record, err := reconcileRemoteInitialPrompt(ctx, client, snapshot, remotePassthroughRecordFor(execution))
	if err != nil {
		return err
	}
	if err := m.saveRemotePassthroughRecord(ctx, execution, record); err != nil {
		return err
	}
	if record.InitialPromptState == remotePromptWritten {
		execution.passthroughInitialPromptProcessID = ""
	}
	return nil
}

func (m *Manager) adoptRemotePassthrough(execution *AgentExecution, snapshot types.PassthroughSnapshot) {
	execution.remotePassthroughMu.Lock()
	execution.remotePassthrough.snapshot = snapshot
	execution.remotePassthroughMu.Unlock()
	execution.PassthroughProcessID = snapshot.ProcessID
	execution.PassthroughStartedAt = snapshot.StartedAt
	execution.isResumedSession = true
	execution.Status = v1.AgentStatusRunning
	record := remotePassthroughRecordFor(execution)
	if record.InitialPromptState == remotePromptPending || record.InitialPromptState == remotePromptUncertain {
		execution.passthroughInitialPromptProcessID = snapshot.ProcessID
		return
	}
	if snapshot.WaitingForInput {
		execution.Status = v1.AgentStatusReady
	}
}
