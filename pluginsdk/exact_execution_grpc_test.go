package pluginsdk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type exactExecutionHostFixture struct {
	exactHostFixture
	execution   *exactExecutionManagerFixture
	interaction *exactInteractionManagerFixture
}

func (h *exactExecutionHostFixture) ExecutionCommands() ExactExecutionCommandManager {
	if h.execution == nil {
		h.execution = &exactExecutionManagerFixture{}
	}
	return h.execution
}

func (h *exactExecutionHostFixture) ExactInteractionCommands() ExactInteractionCommandManager {
	if h.interaction == nil {
		h.interaction = &exactInteractionManagerFixture{}
	}
	return h.interaction
}

type exactExecutionManagerFixture struct {
	ensure     ExactTaskRunCommand
	stop       ExactTaskExecutionCommand
	cancelTurn ExactTaskExecutionCommand
	recover    ExactSessionRecoveryCommand
	cancel     ExactPendingTransitionCommand
	modeRead   ExactTaskExecutionCommand
	modeSet    ExactSessionModeCommand
	stepRead   ExactStepApprovalQuery
	stepWrite  ExactStepApprovalCommand
}

func (f *exactExecutionManagerFixture) EnsureTaskRun(_ context.Context, in ExactTaskRunCommand) (*CommandResult, ExactTaskRunResult, error) {
	f.ensure = in
	return &CommandResult{Status: CommandApplied}, ExactTaskRunResult{SessionID: "session-1", SessionState: "RUNNING", ExecutionID: "execution-1"}, nil
}

func (f *exactExecutionManagerFixture) StopTaskRun(_ context.Context, in ExactTaskExecutionCommand) (*CommandResult, bool, error) {
	f.stop = in
	return &CommandResult{Status: CommandApplied}, true, nil
}

func (f *exactExecutionManagerFixture) CancelTurn(_ context.Context, in ExactTaskExecutionCommand) (*CommandResult, bool, error) {
	f.cancelTurn = in
	return &CommandResult{Status: CommandApplied}, true, nil
}

func (f *exactExecutionManagerFixture) RecoverSession(_ context.Context, in ExactSessionRecoveryCommand) (*CommandResult, ExactTaskRunResult, error) {
	f.recover = in
	return &CommandResult{Status: CommandApplied}, ExactTaskRunResult{SessionID: in.SessionID, ExecutionID: "execution-2"}, nil
}

func (f *exactExecutionManagerFixture) CancelPendingTaskTransition(_ context.Context, in ExactPendingTransitionCommand) (*CommandResult, error) {
	f.cancel = in
	return &CommandResult{Status: CommandApplied}, nil
}

func (f *exactExecutionManagerFixture) GetSessionModeContext(_ context.Context, in ExactTaskExecutionCommand) (SessionModeContext, error) {
	f.modeRead = in
	return SessionModeContext{SessionID: in.SessionID, ExecutionID: "execution-1", CurrentModeID: "plan", AvailableModes: []SessionModeOption{{ID: "plan", Name: "Plan"}}}, nil
}

func (f *exactExecutionManagerFixture) SetSessionMode(_ context.Context, in ExactSessionModeCommand) (*CommandResult, string, error) {
	f.modeSet = in
	return &CommandResult{Status: CommandApplied}, in.ModeID, nil
}

func (f *exactExecutionManagerFixture) GetStepApproval(_ context.Context, in ExactStepApprovalQuery) (StepApproval, error) {
	f.stepRead = in
	return StepApproval{
		SessionID: in.SessionID, TaskID: "task-1", Awaiting: true, FromStepID: "review", FromStepName: "Review",
		ToStepID: "done", ToStepName: "Done", ResourceVersion: "sha256:step", ObservedAt: "2026-11-22T10:00:00Z",
	}, nil
}

func (f *exactExecutionManagerFixture) ApproveStep(_ context.Context, in ExactStepApprovalCommand) (*CommandResult, StepApprovalResult, error) {
	f.stepWrite = in
	return &CommandResult{Status: CommandApplied}, StepApprovalResult{FromStepID: "review", ToStepID: "done"}, nil
}

type exactInteractionManagerFixture struct {
	permission ExactPermissionResponse
	answer     ExactClarificationResponse
}

func (f *exactInteractionManagerFixture) RespondPermission(_ context.Context, in ExactPermissionResponse) (*CommandResult, *Interaction, error) {
	f.permission = in
	return &CommandResult{Status: CommandApplied}, &Interaction{ID: in.InteractionID, Status: InteractionStatusApproved}, nil
}

func (f *exactInteractionManagerFixture) AnswerClarification(_ context.Context, in ExactClarificationResponse) (*CommandResult, *Interaction, error) {
	f.answer = in
	return &CommandResult{Status: CommandApplied}, &Interaction{ID: in.InteractionID, Status: InteractionStatusAnswered}, nil
}

func TestExactExecutionAndInteractionCommandsRoundTrip(t *testing.T) {
	fixture := &exactExecutionHostFixture{}
	host := dialHostOverBufconn(t, fixture)
	execution, ok := HostExecutionCommands(host)
	require.True(t, ok)
	interactions, ok := HostExactInteractionCommands(host)
	require.True(t, ok)

	base := ExactTaskExecutionCommand{
		RequestID: "request-1", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		ExpectedSessionResourceVersion: "session-version", ExpectedExecutionID: "execution-1",
		IdempotencyKey: "operation-1", ManagementInstanceKey: "delivery-lead", ExpectedClaimGeneration: 7,
		ApprovalRevision: 7, ManifestDigest: "manifest-digest",
	}
	ensure := ExactTaskRunCommand{
		RequestID: "request-ensure", WorkspaceID: "workspace-1", TaskID: "task-1",
		ExpectedTaskResourceVersion: "task-version", IdempotencyKey: "ensure-1",
		ManagementInstanceKey: "delivery-lead", ExpectedClaimGeneration: 7, ApprovalRevision: 7,
		ManifestDigest: "manifest-digest",
	}
	result, run, err := execution.EnsureTaskRun(context.Background(), ensure)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, "execution-1", run.ExecutionID)
	require.Equal(t, ensure, fixture.execution.ensure)

	result, stopped, err := execution.StopTaskRun(context.Background(), base)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.True(t, stopped)
	require.Equal(t, base, fixture.execution.stop)

	result, cancelled, err := execution.CancelTurn(context.Background(), base)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.True(t, cancelled)
	require.Equal(t, base, fixture.execution.cancelTurn)

	recovery := ExactSessionRecoveryCommand{ExactTaskExecutionCommand: base, Action: "resume"}
	_, recovered, err := execution.RecoverSession(context.Background(), recovery)
	require.NoError(t, err)
	require.Equal(t, "execution-2", recovered.ExecutionID)
	require.Equal(t, recovery, fixture.execution.recover)

	pending := ExactPendingTransitionCommand{
		RequestID: "request-cancel", WorkspaceID: "workspace-1", TaskID: "task-1", TransitionID: "transition-1",
		ExpectedResourceVersion: "transition-version", IdempotencyKey: "cancel-1",
		ManagementInstanceKey: "delivery-lead", ExpectedClaimGeneration: 7, ApprovalRevision: 7,
		ManifestDigest: "manifest-digest",
	}
	_, err = execution.CancelPendingTaskTransition(context.Background(), pending)
	require.NoError(t, err)
	require.Equal(t, pending, fixture.execution.cancel)

	_, err = execution.GetSessionModeContext(context.Background(), base)
	require.NoError(t, err)
	require.Equal(t, ExactTaskExecutionCommand{
		RequestID: base.RequestID, WorkspaceID: base.WorkspaceID, TaskID: base.TaskID, SessionID: base.SessionID,
		ManagementInstanceKey: base.ManagementInstanceKey, ExpectedClaimGeneration: base.ExpectedClaimGeneration,
		ApprovalRevision: base.ApprovalRevision, ManifestDigest: base.ManifestDigest,
	}, fixture.execution.modeRead)
	mode := ExactSessionModeCommand{ExactTaskExecutionCommand: base, ModeID: "plan"}
	_, current, err := execution.SetSessionMode(context.Background(), mode)
	require.NoError(t, err)
	require.Equal(t, "plan", current)
	require.Equal(t, mode, fixture.execution.modeSet)

	stepQuery := ExactStepApprovalQuery{RequestID: "request-step", WorkspaceID: "workspace-1", SessionID: "session-1"}
	approval, err := execution.GetStepApproval(context.Background(), stepQuery)
	require.NoError(t, err)
	require.Equal(t, stepQuery, fixture.execution.stepRead)
	require.Equal(t, StepApproval{
		SessionID: "session-1", TaskID: "task-1", Awaiting: true, FromStepID: "review", FromStepName: "Review",
		ToStepID: "done", ToStepName: "Done", ResourceVersion: "sha256:step", ObservedAt: "2026-11-22T10:00:00Z",
	}, approval)
	stepCommand := ExactStepApprovalCommand{
		RequestID: "request-approve", WorkspaceID: "workspace-1", SessionID: "session-1",
		ExpectedResourceVersion: "sha256:step", IdempotencyKey: "approve-1", HumanResponseReceiptID: "receipt-step",
		ApprovalRevision: 7, ManifestDigest: "manifest-digest",
	}
	result, approved, err := execution.ApproveStep(context.Background(), stepCommand)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, StepApprovalResult{FromStepID: "review", ToStepID: "done"}, approved)
	require.Equal(t, stepCommand, fixture.execution.stepWrite)

	permission := ExactPermissionResponse{
		RequestID: "request-permission", WorkspaceID: "workspace-1", InteractionID: "interaction-1",
		ExpectedResourceVersion: "interaction-version", OptionID: "allow", HumanResponseReceiptID: "receipt-1",
		ApprovalRevision: 7, ManifestDigest: "manifest-digest",
	}
	_, resolved, err := interactions.RespondPermission(context.Background(), permission)
	require.NoError(t, err)
	require.Equal(t, InteractionStatusApproved, resolved.Status)
	require.Equal(t, permission, fixture.interaction.permission)

	answer := ExactClarificationResponse{
		RequestID: "request-answer", WorkspaceID: "workspace-1", InteractionID: "interaction-2",
		ExpectedResourceVersion: "clarification-version", Answers: []ClarificationAnswer{{QuestionID: "question-1", CustomText: "yes"}},
		HumanResponseReceiptID: "receipt-2", ApprovalRevision: 7, ManifestDigest: "manifest-digest",
	}
	_, answered, err := interactions.AnswerClarification(context.Background(), answer)
	require.NoError(t, err)
	require.Equal(t, InteractionStatusAnswered, answered.Status)
	require.Equal(t, answer, fixture.interaction.answer)
}
