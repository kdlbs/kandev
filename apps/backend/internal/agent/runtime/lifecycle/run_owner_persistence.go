package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

const runExecutionOwnerMetadataKey = "execution_owner"

// Runtime inventory and executor correlation use the durable owner's session.
// AgentExecution.SessionID remains exclusively a task-session identity.
func executionInventorySessionID(execution *AgentExecution) string {
	if execution.Owner.Kind == ExecutionOwnerRun {
		return execution.Owner.RunSessionID
	}
	return execution.SessionID
}

func launchInventorySessionID(req *LaunchRequest) string {
	if req.Owner.Kind == ExecutionOwnerRun {
		return req.Owner.RunSessionID
	}
	return req.SessionID
}

// StopRunOwnerForRecovery proves the predecessor has stopped using its durable
// runtime inventory. A missing in-memory execution is never liveness evidence.
func (m *Manager) StopRunOwnerForRecovery(ctx context.Context, owner ExecutionOwner) error {
	if owner.Kind != ExecutionOwnerRun || owner.RunSessionID == "" {
		return errors.New("invalid run recovery owner")
	}
	if err := m.recoveryGuard.CheckLaunchAllowed(owner.RunSessionID); err != nil {
		return err
	}
	reader, ok := m.runningWriter.(executorRunningReader)
	if !ok {
		return errors.New("run recovery requires durable runtime inventory")
	}
	row, err := reader.GetExecutorRunningBySessionID(ctx, owner.RunSessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) || (err == nil && row == nil) {
		return m.runRecoveryErr
	}
	if err != nil {
		return err
	}
	if err := validateRunRecoveryOwner(row, owner); err != nil {
		return err
	}
	if row.Status == models.ExecutorRunningStatusStopped {
		return nil
	}
	backend, err := m.executorRegistry.GetBackend(row.Runtime)
	if err != nil {
		return err
	}
	instance := &ExecutorInstance{
		InstanceID: row.AgentExecutionID, SessionID: row.SessionID,
		StandaloneInstanceID: row.AgentExecutionID, StandalonePort: row.AgentctlPort,
		ContainerID: row.ContainerID, RuntimeName: row.Runtime, Metadata: row.Metadata,
		WorkspacePath: row.WorktreePath, StopReason: "office_run_recovery",
	}
	if err := backend.StopInstance(ctx, instance, true); err != nil {
		return fmt.Errorf("stop predecessor run execution: %w", err)
	}
	return m.runningWriter.RepairExecutorRunningDead(ctx, row.SessionID)
}

func validateRunRecoveryOwner(row *models.ExecutorRunning, owner ExecutionOwner) error {
	var recorded ExecutionOwner
	raw, err := json.Marshal(row.Metadata[runExecutionOwnerMetadataKey])
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &recorded); err != nil {
		return err
	}
	if recorded != owner || row.TaskID != "" || row.AgentExecutionID == "" {
		return errors.New("run recovery inventory owner mismatch")
	}
	return nil
}
