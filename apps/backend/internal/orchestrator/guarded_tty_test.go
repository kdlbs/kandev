package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type guardedTTYAgentManager struct {
	*mockAgentManager
	sequence     *[]string
	request      models.GuardedTTYDispatchRequest
	receipt      *streams.GuardedTTYExecReceipt
	err          error
	beforeReturn func()
}

func (m *guardedTTYAgentManager) ExecuteGuardedTTY(
	_ context.Context,
	request models.GuardedTTYDispatchRequest,
) (*streams.GuardedTTYExecReceipt, error) {
	*m.sequence = append(*m.sequence, "dispatch")
	m.request = request
	if m.beforeReturn != nil {
		m.beforeReturn()
	}
	return m.receipt, m.err
}

type guardedTTYAuditRecorderStub struct {
	*mockMessageCreator
	sequence           *[]string
	claim              models.GuardedTTYAuditClaim
	finalize           models.GuardedTTYAuditFinalize
	claimErr           error
	finalizeErr        error
	finalizeContextErr error
}

func (r *guardedTTYAuditRecorderStub) ClaimGuardedTTYExecution(_ context.Context, claim models.GuardedTTYAuditClaim) error {
	*r.sequence = append(*r.sequence, "claim")
	r.claim = claim
	return r.claimErr
}

func (r *guardedTTYAuditRecorderStub) FinalizeGuardedTTYExecution(ctx context.Context, finalize models.GuardedTTYAuditFinalize) error {
	*r.sequence = append(*r.sequence, "finalize")
	r.finalize = finalize
	r.finalizeContextErr = ctx.Err()
	return r.finalizeErr
}

func TestExecuteGuardedTTYClaimsDispatchesAndFinalizesExactReceipt(t *testing.T) {
	sequence := []string{}
	completedAt := time.Date(2026, 8, 30, 12, 0, 1, 0, time.UTC)
	manager := &guardedTTYAgentManager{
		mockAgentManager: &mockAgentManager{},
		sequence:         &sequence,
	}
	recorder := &guardedTTYAuditRecorderStub{
		mockMessageCreator: &mockMessageCreator{},
		sequence:           &sequence,
	}
	svc := &Service{agentManager: manager, messageCreator: recorder, logger: testLogger()}
	request := streams.GuardedTTYExecRequest{
		Execution: streams.MCPExecutionContext{ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1"},
		Argv:      []string{"stty", "-a"},
	}
	manager.receipt = &streams.GuardedTTYExecReceipt{
		ContractVersion: 1,
		BridgeVersion:   "1.8.0",
		ACPSessionID:    "acp-session-1",
		ExecutionID:     "execution-1",
		TaskID:          "task-1",
		SessionID:       "session-1",
		Method:          streams.GuardedTTYExecMethod,
		RequestedTTY:    true,
		DispatchedTTY:   true,
		ProcessID:       "provider-process-1",
		CWD:             "/workspace/task",
		Stdout:          "SECRET_OUTPUT_CANARY",
		StdoutBytes:     20,
		Output:          "SECRET_OUTPUT_CANARY",
		OutputBytes:     20,
		OutputSHA256:    "provider-supplied-digest-is-not-trusted",
		ExitCode:        0,
		Outcome:         string(models.GuardedTTYOutcomeSucceeded),
		CompletionCount: 1,
		StartedAt:       completedAt.Add(-time.Second),
		CompletedAt:     completedAt,
	}

	receipt, err := svc.ExecuteGuardedTTY(guardedTTYOrchestratorContext(), request)

	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.Equal(t, []string{"claim", "dispatch", "finalize"}, sequence)
	assert.NotEmpty(t, recorder.claim.AttestationID)
	assert.Equal(t, recorder.claim.AttestationID, manager.request.AttestationID)
	assert.Equal(t, recorder.claim.AttestationID, receipt.AttestationID)
	assert.Equal(t, request.Execution, manager.request.Execution)
	assert.Equal(t, request.Argv, manager.request.Argv)
	assert.Equal(t, "workspace-1", recorder.claim.WorkspaceID)
	assert.Equal(t, "user-1", recorder.claim.ActorUserID)
	assert.Equal(t, streams.GuardedTTYAgentID, recorder.claim.AgentID)
	assert.Equal(t, string(mcpprofile.SurfaceKanbanTask), recorder.claim.PrincipalSurface)
	assert.Equal(t, models.GuardedTTYOutcomeSucceeded, recorder.finalize.Outcome)
	assert.Equal(t, manager.receipt.StartedAt, recorder.finalize.ProviderMetadata["started_at"])
	assert.Equal(t, "519a5e3aec51fd548c2f9f53651f57ed9f4fd9c59b304a434a05e43e4aaf15c4", recorder.finalize.OutputSHA256)
	assert.NotContains(t, recorder.finalize.ProviderMetadata, "SECRET_OUTPUT_CANARY")
}

func TestExecuteGuardedTTYFinalizesAfterCallerCancellation(t *testing.T) {
	sequence := []string{}
	completedAt := time.Date(2026, 8, 30, 12, 0, 1, 0, time.UTC)
	ctx, cancel := context.WithCancel(guardedTTYOrchestratorContext())
	manager := &guardedTTYAgentManager{
		mockAgentManager: &mockAgentManager{},
		sequence:         &sequence,
		beforeReturn:     cancel,
		receipt: &streams.GuardedTTYExecReceipt{
			ContractVersion: 1, BridgeVersion: "1.8.0", ACPSessionID: "acp-session-1",
			ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1",
			Method: streams.GuardedTTYExecMethod, RequestedTTY: true, DispatchedTTY: true,
			ProcessID: "process-1", CWD: "/workspace/task", Stdout: "done\n", StdoutBytes: 5,
			Output: "done\n", OutputBytes: 5, Outcome: string(models.GuardedTTYOutcomeSucceeded),
			CompletionCount: 1, StartedAt: completedAt.Add(-time.Second), CompletedAt: completedAt,
		},
	}
	recorder := &guardedTTYAuditRecorderStub{mockMessageCreator: &mockMessageCreator{}, sequence: &sequence}
	svc := &Service{agentManager: manager, messageCreator: recorder, logger: testLogger()}

	receipt, err := svc.ExecuteGuardedTTY(ctx, streams.GuardedTTYExecRequest{
		Execution: streams.MCPExecutionContext{ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1"},
		Argv:      []string{"pwd"},
	})

	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.Equal(t, []string{"claim", "dispatch", "finalize"}, sequence)
	assert.NoError(t, recorder.finalizeContextErr, "audit finalization must outlive caller cancellation")
}

func TestExecuteGuardedTTYFinalizesStableBridgeDenials(t *testing.T) {
	tests := []struct {
		denialCode string
		dispatched bool
		want       models.GuardedTTYOutcome
	}{
		{denialCode: "stale_session", want: models.GuardedTTYOutcomeStale},
		{denialCode: "timeout", dispatched: true, want: models.GuardedTTYOutcomeTimedOut},
		{denialCode: "cancelled", dispatched: true, want: models.GuardedTTYOutcomeCancelled},
		{denialCode: "output_overflow", dispatched: true, want: models.GuardedTTYOutcomeOverflow},
		{denialCode: "invalid_output", dispatched: true, want: models.GuardedTTYOutcomeProviderFailure},
		{denialCode: "app_server_error", dispatched: true, want: models.GuardedTTYOutcomeProviderFailure},
	}
	for _, tt := range tests {
		t.Run(tt.denialCode, func(t *testing.T) {
			sequence := []string{}
			completedAt := time.Date(2026, 8, 30, 12, 0, 1, 0, time.UTC)
			manager := &guardedTTYAgentManager{
				mockAgentManager: &mockAgentManager{},
				sequence:         &sequence,
				receipt: &streams.GuardedTTYExecReceipt{
					ContractVersion: 1, BridgeVersion: "1.8.0", ACPSessionID: "acp-session-1",
					ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1",
					Method: streams.GuardedTTYExecMethod, RequestedTTY: true, DispatchedTTY: tt.dispatched,
					Outcome: tt.denialCode, DenialCode: tt.denialCode, CompletionCount: 1,
					StartedAt: completedAt.Add(-time.Second), CompletedAt: completedAt,
				},
			}
			if tt.dispatched {
				manager.receipt.ProcessID = "process-1"
				manager.receipt.CWD = "/workspace/task"
			}
			recorder := &guardedTTYAuditRecorderStub{mockMessageCreator: &mockMessageCreator{}, sequence: &sequence}
			svc := &Service{agentManager: manager, messageCreator: recorder, logger: testLogger()}

			receipt, err := svc.ExecuteGuardedTTY(guardedTTYOrchestratorContext(), streams.GuardedTTYExecRequest{
				Execution: streams.MCPExecutionContext{ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1"},
				Argv:      []string{"stty"},
			})

			require.Error(t, err)
			assert.Nil(t, receipt)
			assert.Equal(t, []string{"claim", "dispatch", "finalize"}, sequence)
			assert.Equal(t, tt.want, recorder.finalize.Outcome)
			assert.Equal(t, tt.dispatched, recorder.finalize.ProviderMetadata["dispatched_tty"])
		})
	}
}

func TestExecuteGuardedTTYDoesNotDispatchWithoutDurableClaim(t *testing.T) {
	sequence := []string{}
	manager := &guardedTTYAgentManager{mockAgentManager: &mockAgentManager{}, sequence: &sequence}
	recorder := &guardedTTYAuditRecorderStub{
		mockMessageCreator: &mockMessageCreator{}, sequence: &sequence, claimErr: errors.New("database unavailable"),
	}
	svc := &Service{agentManager: manager, messageCreator: recorder, logger: testLogger()}

	receipt, err := svc.ExecuteGuardedTTY(guardedTTYOrchestratorContext(), streams.GuardedTTYExecRequest{
		Execution: streams.MCPExecutionContext{ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1"},
		Argv:      []string{"stty"},
	})

	require.Error(t, err)
	assert.Nil(t, receipt)
	assert.Equal(t, []string{"claim"}, sequence)
}

func TestExecuteGuardedTTYRecordsUnknownDispatchStateOnTransportFailure(t *testing.T) {
	sequence := []string{}
	manager := &guardedTTYAgentManager{
		mockAgentManager: &mockAgentManager{}, sequence: &sequence,
		err: errors.New("transport failed with SECRET_CANARY"),
	}
	recorder := &guardedTTYAuditRecorderStub{mockMessageCreator: &mockMessageCreator{}, sequence: &sequence}
	svc := &Service{agentManager: manager, messageCreator: recorder, logger: testLogger()}

	receipt, err := svc.ExecuteGuardedTTY(guardedTTYOrchestratorContext(), streams.GuardedTTYExecRequest{
		Execution: streams.MCPExecutionContext{ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1"},
		Argv:      []string{"pwd"},
	})

	require.Error(t, err)
	assert.Nil(t, receipt)
	assert.Equal(t, []string{"claim", "dispatch", "finalize"}, sequence)
	assert.Equal(t, models.GuardedTTYOutcomeProviderFailure, recorder.finalize.Outcome)
	assert.Nil(t, recorder.finalize.ProviderMetadata["dispatched_tty"])
	encoded, marshalErr := json.Marshal(recorder.finalize)
	require.NoError(t, marshalErr)
	assert.NotContains(t, string(encoded), "SECRET_CANARY")
}

func TestExecuteGuardedTTYFailsClosedOnMismatchedProviderReceipt(t *testing.T) {
	sequence := []string{}
	manager := &guardedTTYAgentManager{
		mockAgentManager: &mockAgentManager{}, sequence: &sequence,
		receipt: &streams.GuardedTTYExecReceipt{
			ExecutionID: "execution-other", TaskID: "task-1", SessionID: "session-1",
			Method: streams.GuardedTTYExecMethod, RequestedTTY: true, DispatchedTTY: true,
			CompletionCount: 1, CompletedAt: time.Now().UTC(),
		},
	}
	recorder := &guardedTTYAuditRecorderStub{mockMessageCreator: &mockMessageCreator{}, sequence: &sequence}
	svc := &Service{agentManager: manager, messageCreator: recorder, logger: testLogger()}

	receipt, err := svc.ExecuteGuardedTTY(guardedTTYOrchestratorContext(), streams.GuardedTTYExecRequest{
		Execution: streams.MCPExecutionContext{ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1"},
		Argv:      []string{"stty"},
	})

	require.Error(t, err)
	assert.Nil(t, receipt)
	assert.Equal(t, []string{"claim", "dispatch", "finalize"}, sequence)
	assert.Equal(t, models.GuardedTTYOutcomeProviderFailure, recorder.finalize.Outcome)
}

func TestExecuteGuardedTTYRejectsSuccessWithoutCompleteProviderEvidence(t *testing.T) {
	sequence := []string{}
	completedAt := time.Date(2026, 8, 30, 12, 0, 1, 0, time.UTC)
	manager := &guardedTTYAgentManager{
		mockAgentManager: &mockAgentManager{}, sequence: &sequence,
		receipt: &streams.GuardedTTYExecReceipt{
			ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1",
			Method: streams.GuardedTTYExecMethod, RequestedTTY: true, DispatchedTTY: true,
			Outcome: string(models.GuardedTTYOutcomeSucceeded), CompletionCount: 1,
			StartedAt: completedAt.Add(-time.Second), CompletedAt: completedAt,
		},
	}
	recorder := &guardedTTYAuditRecorderStub{mockMessageCreator: &mockMessageCreator{}, sequence: &sequence}
	svc := &Service{agentManager: manager, messageCreator: recorder, logger: testLogger()}

	receipt, err := svc.ExecuteGuardedTTY(guardedTTYOrchestratorContext(), streams.GuardedTTYExecRequest{
		Execution: streams.MCPExecutionContext{ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1"},
		Argv:      []string{"stty"},
	})

	require.Error(t, err)
	assert.Nil(t, receipt)
	assert.Equal(t, []string{"claim", "dispatch", "finalize"}, sequence)
	assert.Equal(t, models.GuardedTTYOutcomeProviderFailure, recorder.finalize.Outcome)
}

func TestExecuteGuardedTTYRejectsMissingTrustedPrincipalBeforeAuditOrDispatch(t *testing.T) {
	sequence := []string{}
	manager := &guardedTTYAgentManager{mockAgentManager: &mockAgentManager{}, sequence: &sequence}
	recorder := &guardedTTYAuditRecorderStub{mockMessageCreator: &mockMessageCreator{}, sequence: &sequence}
	svc := &Service{agentManager: manager, messageCreator: recorder, logger: testLogger()}

	receipt, err := svc.ExecuteGuardedTTY(context.Background(), streams.GuardedTTYExecRequest{
		Execution: streams.MCPExecutionContext{ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1"},
		Argv:      []string{"stty"},
	})

	require.Error(t, err)
	assert.Nil(t, receipt)
	assert.Empty(t, sequence)
}

func guardedTTYOrchestratorContext() context.Context {
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-1", Role: authn.RoleMember})
	return mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		WorkspaceID: "workspace-1", CallerTaskID: "task-1", CallerSessionID: "session-1",
		Surface: mcpprofile.SurfaceKanbanTask,
	})
}
