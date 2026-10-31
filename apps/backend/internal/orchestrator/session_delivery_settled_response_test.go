package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type settlingPromptRecoveryManager struct {
	*mockAgentManager
	during func()
	result *lifecycle.DeliveryReconciliationResult
}

func (m *settlingPromptRecoveryManager) RecoverAgentPromptStreamWithIdentity(context.Context, lifecycle.AgentDeliveryRecoveryIdentity) lifecycle.DeliveryReconciliationResult {
	m.during()
	if m.result != nil {
		return *m.result
	}
	return lifecycle.DeliveryReconciliationResult{Outcome: lifecycle.DeliveryReconciliationOwnerMismatch, Err: lifecycle.ErrDeliveryOwnerMismatch}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
func TestRetrySessionDeliveryReportsConcurrentSettlement(t *testing.T) {
	ctx := context.Background()
	f := newRetryTerminalFixture(t)
	manager := &settlingPromptRecoveryManager{mockAgentManager: &mockAgentManager{}, during: f.settle}
	f.service.agentManager = manager
	response, err := f.service.RetrySessionDelivery(ctx, f.session.TaskID, f.session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoverySettled, response.Outcome, response.Reason)
	require.Empty(t, response.Reason)
	latest, err := f.repo.GetTaskSession(ctx, f.session.ID)
	require.NoError(t, err)
	recovery, ok := models.LoadAgentDeliveryRecovery(latest.Metadata)
	require.True(t, ok)
	require.Equal(t, recovery.Revision, response.RecoveryRevision)
	require.Equal(t, f.recovery.SubmissionID, response.RecoveryIdentity.SubmissionID)
	require.Empty(t, manager.capturedPromptCalls)
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
func TestRetrySessionDeliveryConcurrentSettlementKeepsFences(t *testing.T) {
	tests := []struct {
		name   string
		change func(*testing.T, *retryTerminalFixture)
	}{
		{"no terminal proof", func(*testing.T, *retryTerminalFixture) {}},
		{"independent block", func(t *testing.T, f *retryTerminalFixture) {
			f.settle()
			require.NoError(t, f.repo.UpsertSessionRecoveryBlock(context.Background(), &models.SessionRecoveryBlock{ID: "independent", SessionID: f.session.ID, IncarnationID: f.session.QueueIncarnationID, ExpectedGeneration: 1, Reason: "mcp_auth_required", ConsumerReference: "mcp", State: models.RecoveryBlockOpen}))
		}},
		{"new submission", func(t *testing.T, f *retryTerminalFixture) {
			f.settle()
			_, err := f.repo.PrepareAgentDeliverySubmission(context.Background(), &models.AgentDeliverySubmission{ID: "new-submission", SessionID: f.session.ID, IncarnationID: f.session.QueueIncarnationID, HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "new-hash", Payload: []byte("new instruction"), State: models.DeliverySubmissionDispatching})
			require.NoError(t, err)
			next := f.recovery
			next.SubmissionID = "new-submission"
			next.PromptGeneration = 2
			changed, err := f.repo.UpsertAgentDeliveryRecovery(context.Background(), &next, &models.SessionRecoveryBlock{ID: "new-delivery-block", SessionID: f.session.ID, IncarnationID: f.session.QueueIncarnationID, ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", ConsumerReference: "agent_delivery", DeliverySubmissionID: next.SubmissionID, DeliveryStreamID: next.StreamID, State: models.RecoveryBlockOpen})
			require.NoError(t, err)
			require.True(t, changed)
		}},

		{"new generation", func(t *testing.T, f *retryTerminalFixture) {
			f.settle()
			require.NoError(t, f.repo.CreateHarnessSessionGeneration(context.Background(), &models.HarnessSessionGeneration{SessionID: f.session.ID, IncarnationID: f.session.QueueIncarnationID, Generation: 2, NativeSessionID: "replacement-native", CreationReason: "restart"}))
		}},
		{"new execution", func(t *testing.T, f *retryTerminalFixture) {
			f.settle()
			running, err := f.repo.GetExecutorRunningBySessionID(context.Background(), f.session.ID)
			require.NoError(t, err)
			running.AgentExecutionID = "replacement-execution"
			require.NoError(t, f.repo.UpsertExecutorRunning(context.Background(), running))
			current, err := f.repo.GetTaskSession(context.Background(), f.session.ID)
			require.NoError(t, err)
			require.Equal(t, "replacement-execution", current.AgentExecutionID)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRetryTerminalFixture(t)
			manager := &settlingPromptRecoveryManager{mockAgentManager: &mockAgentManager{}, during: func() { tt.change(t, f) }}
			f.service.agentManager = manager
			response, err := f.service.RetrySessionDelivery(context.Background(), f.session.TaskID, f.session.ID)
			require.NoError(t, err)
			require.Equal(t, SessionDeliveryRecoveryBlocked, response.Outcome)
			require.Empty(t, manager.capturedPromptCalls)
			if tt.name == "independent block" {
				block, err := f.repo.GetOpenSessionRecoveryBlock(context.Background(), f.session.ID, f.session.QueueIncarnationID, 1)
				require.NoError(t, err)
				require.NotNil(t, block)
				require.Equal(t, "mcp_auth_required", block.Reason)
			}
		})
	}
}

type retrySettlementReadFailure struct {
	*tasksqlite.Repository
	unavailable bool
}

func (r *retrySettlementReadFailure) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	if r.unavailable {
		return nil, errors.New("canonical evidence unavailable")
	}
	return r.Repository.GetTaskSession(ctx, id)
}

func TestRetrySessionDeliveryConcurrentSettlementRequiresReadableEvidence(t *testing.T) {
	f := newRetryTerminalFixture(t)
	store := &retrySettlementReadFailure{Repository: f.repo}
	f.service.repo = store
	manager := &settlingPromptRecoveryManager{mockAgentManager: &mockAgentManager{}, during: func() { f.settle(); store.unavailable = true }}
	f.service.agentManager = manager
	response, err := f.service.RetrySessionDelivery(context.Background(), f.session.TaskID, f.session.ID)
	require.Error(t, err)
	require.Nil(t, response)
	require.Empty(t, manager.capturedPromptCalls)
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
func TestRetrySessionDeliveryDirectSettlementReturnsCanonicalRevision(t *testing.T) {
	ctx := context.Background()
	f := newRetryTerminalFixture(t)
	manager := &settlingPromptRecoveryManager{
		mockAgentManager: &mockAgentManager{}, during: f.settle,
		result: &lifecycle.DeliveryReconciliationResult{Outcome: lifecycle.DeliveryReconciliationTerminalSettled},
	}
	f.service.agentManager = manager
	response, err := f.service.RetrySessionDelivery(ctx, f.session.TaskID, f.session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoverySettled, response.Outcome, response.Reason)
	latest, err := f.repo.GetTaskSession(ctx, f.session.ID)
	require.NoError(t, err)
	recovery, ok := models.LoadAgentDeliveryRecovery(latest.Metadata)
	require.True(t, ok)
	require.Equal(t, recovery.Revision, response.RecoveryRevision)
	require.Empty(t, manager.capturedPromptCalls)
}
