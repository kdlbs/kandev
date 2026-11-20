package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/common/sshlaunch"
	"github.com/kandev/kandev/internal/task/models"
)

type executionSelectiveFailingStopper struct{ failExecutionID string }

func (executionSelectiveFailingStopper) StopTask(context.Context, string, string, bool) error {
	return nil
}
func (executionSelectiveFailingStopper) RegisterExecutionStopOwner(string, string, bool) {}
func (executionSelectiveFailingStopper) StopSession(context.Context, string, string, bool) error {
	return nil
}
func (s executionSelectiveFailingStopper) StopExecution(_ context.Context, executionID, _ string, _ bool) error {
	if executionID == s.failExecutionID {
		return errors.New("controller reconciliation unresolved")
	}
	return nil
}

func sshCleanupReceiptMetadata(t *testing.T, taskID, sessionID, executionID string) map[string]interface{} {
	t.Helper()
	generation, err := sshlaunch.NewGeneration()
	require.NoError(t, err)
	receipt := sshlaunch.Receipt{
		Version:    sshlaunch.Version,
		Generation: generation,
		Owner: sshlaunch.Owner{
			TaskID:               taskID,
			SessionID:            sessionID,
			ControllerInstanceID: executionID,
			TaskDir:              "/home/kandev/.kandev/tasks/" + taskID,
			SessionDir:           "/home/kandev/.kandev/tasks/" + taskID + "/sessions/" + sessionID,
			Target: sshlaunch.TargetBinding{
				Host:            "runner.example.test",
				Port:            "22",
				User:            "kandev",
				HostFingerprint: "SHA256:test",
			},
		},
		UID: 1000,
		Process: processidentity.Identity{
			PID:        4242,
			GroupID:    4242,
			SessionID:  4242,
			BirthToken: "linux:11111111-2222-3333-4444-555555555555:987654",
		},
	}
	require.NoError(t, receipt.Validate())
	projected, err := receipt.Metadata()
	require.NoError(t, err)
	return map[string]interface{}{
		"ssh_remote_agentctl_pid":         "4242",
		"ssh_remote_session_dir":          receipt.Owner.SessionDir,
		"ssh_remote_task_dir":             receipt.Owner.TaskDir,
		"ssh_remote_agentctl_instance_id": executionID,
		"ssh_host":                        receipt.Owner.Target.Host,
		"ssh_port":                        receipt.Owner.Target.Port,
		"ssh_user":                        receipt.Owner.Target.User,
		"ssh_host_fingerprint":            receipt.Owner.Target.HostFingerprint,
		sshlaunch.MetadataKey:             projected,
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.18
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.23
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHCleanupReconciliationRetainsMixedAndSuccessorTargets(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	receiptMetadata := sshCleanupReceiptMetadata(t, "task-1", "session-ok", "exec-ok")
	legacyMetadata := map[string]interface{}{
		"ssh_remote_agentctl_pid":         "4343",
		"ssh_remote_session_dir":          "/home/kandev/.kandev/tasks/task-1/sessions/session-legacy",
		"ssh_remote_task_dir":             "/home/kandev/.kandev/tasks/task-1",
		"ssh_remote_agentctl_instance_id": "exec-legacy",
		"ssh_host":                        "runner.example.test",
		"ssh_port":                        "22",
		"ssh_user":                        "kandev",
		"ssh_host_fingerprint":            "SHA256:test",
	}
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-ok", SessionID: "session-ok", TaskID: "task-1",
		Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusRunning,
		AgentExecutionID: "exec-ok", Metadata: receiptMetadata,
	}))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-legacy", SessionID: "session-legacy", TaskID: "task-1",
		Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusRunning,
		AgentExecutionID: "exec-legacy", Metadata: legacyMetadata,
	}))

	// Durable cleanup captures the same receipt bytes it will later settle on.
	snapshot := &taskResourceCleanupSnapshot{WorkspaceCleanup: models.WorkspaceCleanupRetain}
	require.NoError(t, svc.captureRetainedRuntimeInventory(ctx, "task-1", snapshot))
	require.Len(t, snapshot.RetainedExecutors, 2)
	raw, err := json.Marshal(snapshot)
	require.NoError(t, err)
	var reloaded taskResourceCleanupSnapshot
	require.NoError(t, json.Unmarshal(raw, &reloaded))

	original, ok := sshlaunch.ReceiptFromMetadata(receiptMetadata)
	require.True(t, ok)
	stored, ok := sshlaunch.ReceiptFromMetadata(executorRowBySession(t, reloaded.RetainedExecutors, "session-ok").Metadata)
	require.True(t, ok, "the persisted cleanup snapshot dropped the launch receipt")
	require.Equal(t, original, stored, "receipt bytes changed across snapshot capture and reload")
	require.NotContains(t,
		executorRowBySession(t, reloaded.RetainedExecutors, "session-legacy").Metadata,
		sshlaunch.MetadataKey,
		"a legacy record must not acquire invented proof")

	// One completed target beside an unresolved legacy target: the completed
	// stop may settle, the unresolved one keeps its inventory and blocks
	// destructive cleanup.
	svc.SetExecutionStopper(executionSelectiveFailingStopper{failExecutionID: "exec-legacy"})
	targets := taskStopTargetsFromRunningRows(reloaded.RetainedExecutors)
	outcome := svc.stopTaskRuntimeTargetsWithTaskDeleted(
		ctx, "task-1", targets, "task deleted", "task cleanup runtime stop failed", true, true)
	_, failed := outcome.failed["session-legacy"]
	require.True(t, failed, "the unresolved target must be recorded as failed")
	_, failedCompleted := outcome.failed["session-ok"]
	require.False(t, failedCompleted, "a completed target must not fail the whole stop")
	preserve := taskCleanupPreserveRows(outcome)
	_, preserved := preserve["session-legacy"]
	require.True(t, preserved, "destructive cleanup must be blocked for the unresolved target")
	_, preservedSuccess := preserve["session-ok"]
	require.False(t, preservedSuccess, "a settled target may proceed to cleanup")

	// The unresolved target keeps its durable evidence.
	legacyRow, err := repo.GetExecutorRunningBySessionID(ctx, "session-legacy")
	require.NoError(t, err)
	require.NotNil(t, legacyRow)
	require.Equal(t, "exec-legacy", legacyRow.AgentExecutionID)

	// A successor inserted before final settlement becomes the new owner; the
	// stale target must not be settled against it.
	successorReceipt := sshCleanupReceiptMetadata(t, "task-1", "session-legacy", "exec-successor")
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-legacy", SessionID: "session-legacy", TaskID: "task-1",
		Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusRunning,
		AgentExecutionID: "exec-successor", Metadata: successorReceipt,
	}))
	refreshed := &taskResourceCleanupSnapshot{WorkspaceCleanup: models.WorkspaceCleanupRetain}
	require.NoError(t, svc.captureRetainedRuntimeInventory(ctx, "task-1", refreshed))
	var successorTarget taskStopTarget
	for _, target := range taskStopTargetsFromRunningRows(refreshed.RetainedExecutors) {
		if target.sessionID == "session-legacy" {
			successorTarget = target
		}
	}
	require.Equal(t, "exec-successor", successorTarget.executionID,
		"cleanup must settle on the current owner, not the stale execution")
	successorReceiptValue, ok := sshlaunch.ReceiptFromMetadata(
		executorRowBySession(t, refreshed.RetainedExecutors, "session-legacy").Metadata)
	require.True(t, ok)
	require.NotEqual(t, original.Generation, successorReceiptValue.Generation,
		"the successor must not inherit the predecessor's launch proof")
}

func executorRowBySession(t *testing.T, rows []*models.ExecutorRunning, sessionID string) *models.ExecutorRunning {
	t.Helper()
	for _, row := range rows {
		if row != nil && row.SessionID == sessionID {
			return row
		}
	}
	t.Fatalf("no executor row for session %q", sessionID)
	return nil
}
