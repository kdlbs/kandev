package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func sshDeliveryRecoveryIdentity(record *models.ExecutorRunning) AgentDeliveryRecoveryIdentity {
	return AgentDeliveryRecoveryIdentity{
		TaskID: record.TaskID, SessionID: record.SessionID, ExecutionID: record.AgentExecutionID,
		SubmissionID: "prompt-ssh", StreamID: "stream-ssh", IncarnationID: "incarnation-ssh",
		HarnessGeneration: 1, PromptGeneration: 1,
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
func TestSSHDeliveryRecoverySavedOwnerMismatchIsBlocked(t *testing.T) {
	_, record, _ := sshDeliveryRecoveryFixture(t)
	saved := *record
	saved.AgentExecutionID = "execution-ssh-successor"
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{prior: &saved})

	result := mgr.RecoverAgentPromptStreamWithIdentity(context.Background(), sshDeliveryRecoveryIdentity(record))

	require.Equal(t, DeliveryReconciliationOwnerMismatch, result.Outcome)
	require.Equal(t, "execution_identity_changed", result.Reason)
	require.ErrorIs(t, result.Err, ErrDeliveryOwnerMismatch)
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
func TestSSHDeliveryRecoveryTrackedOwnerMismatchIsBlocked(t *testing.T) {
	rt, record, _ := sshDeliveryRecoveryFixture(t)
	instances, _, err := rt.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{record})
	require.NoError(t, err)
	require.Len(t, instances, 1)
	instance := instances[0]
	rt.mu.Lock()
	lost := rt.sessions[record.AgentExecutionID]
	rt.mu.Unlock()
	rt.transportTeardown(record.AgentExecutionID, lost, "test_disconnect", 0)

	mgr := newTestManager(t)
	mgr.executorRegistry = NewExecutorRegistry(newTestLogger())
	mgr.executorRegistry.Register(rt)
	mgr.secretStore = &mockSecretStore{store: map[string]string{"test-auth": "refreshed-auth-token"}}
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{prior: record})
	execution := &AgentExecution{ID: record.AgentExecutionID, TaskID: record.TaskID, SessionID: record.SessionID,
		RuntimeName: record.Runtime, WorkspacePath: "/remote/task", agentctl: instance.Client,
		DeliveryMode: DurableDeliveryV1, DeliveryIncarnationID: "incarnation-ssh", DeliveryHarnessGeneration: 1,
		DeliveryStreamID: "stream-ssh", promptGeneration: 1}
	execution.metadata = cloneSSHMetadata(instance.Metadata)
	execution.metadata[MetadataKeySSHRemoteAgentctlPID] = "9999"
	execution.setMetadataValue(MetadataKeyAuthTokenSecret, "test-auth")
	execution.setDeliverySubmissionID("prompt-ssh")
	require.NoError(t, mgr.executionStore.Add(execution))

	result := mgr.RecoverAgentPromptStreamWithIdentity(context.Background(), sshDeliveryRecoveryIdentity(record))

	require.Equal(t, DeliveryReconciliationOwnerMismatch, result.Outcome)
	require.Equal(t, "execution_identity_changed", result.Reason)
	require.ErrorIs(t, result.Err, ErrDeliveryOwnerMismatch)
}
