package pluginsdk

import (
	"context"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type grpcExactExecutionCommandManager struct{ client pluginv1.HostClient }
type grpcExactInteractionCommandManager struct{ client pluginv1.HostClient }

func (h *grpcHostClient) ExecutionCommands() ExactExecutionCommandManager {
	return grpcExactExecutionCommandManager{client: h.client}
}

func (h *grpcHostClient) ExactInteractionCommands() ExactInteractionCommandManager {
	return grpcExactInteractionCommandManager{client: h.client}
}

func (m grpcExactExecutionCommandManager) EnsureTaskRun(ctx context.Context, in ExactTaskRunCommand) (*CommandResult, ExactTaskRunResult, error) {
	resp, err := m.client.EnsureTaskRunExact(ctx, &pluginv1.EnsureTaskRunExactRequest{
		RequestId: in.RequestID, WorkspaceId: in.WorkspaceID, TaskId: in.TaskID,
		IdempotencyKey: in.IdempotencyKey, ExpectedTaskResourceVersion: in.ExpectedTaskResourceVersion,
		ManagementInstanceKey: in.ManagementInstanceKey, ExpectedClaimGeneration: in.ExpectedClaimGeneration,
		ApprovalRevision: in.ApprovalRevision, ManifestDigest: in.ManifestDigest,
	})
	if err != nil {
		return nil, ExactTaskRunResult{}, err
	}
	return commandResultFromProto(resp.GetResult()), ExactTaskRunResult{
		SessionID: resp.GetSessionId(), SessionState: resp.GetSessionState(), ExecutionID: resp.GetExecutionId(),
	}, nil
}

func (m grpcExactExecutionCommandManager) StopTaskRun(ctx context.Context, in ExactTaskExecutionCommand) (*CommandResult, bool, error) {
	resp, err := m.client.StopTaskRunExact(ctx, stopTaskRunToProto(in))
	if err != nil {
		return nil, false, err
	}
	return commandResultFromProto(resp.GetResult()), resp.GetStopped(), nil
}

func (m grpcExactExecutionCommandManager) RecoverSession(ctx context.Context, in ExactSessionRecoveryCommand) (*CommandResult, ExactTaskRunResult, error) {
	resp, err := m.client.RecoverSessionExact(ctx, &pluginv1.RecoverSessionExactRequest{
		RequestId: in.RequestID, WorkspaceId: in.WorkspaceID, TaskId: in.TaskID, SessionId: in.SessionID,
		ExpectedSessionResourceVersion: in.ExpectedSessionResourceVersion, ExpectedExecutionId: in.ExpectedExecutionID,
		Action: in.Action, IdempotencyKey: in.IdempotencyKey, ApprovalRevision: in.ApprovalRevision,
		ManifestDigest: in.ManifestDigest, ManagementInstanceKey: in.ManagementInstanceKey,
		ExpectedClaimGeneration: in.ExpectedClaimGeneration,
	})
	if err != nil {
		return nil, ExactTaskRunResult{}, err
	}
	return commandResultFromProto(resp.GetResult()), ExactTaskRunResult{SessionID: resp.GetSessionId(), ExecutionID: resp.GetExecutionId()}, nil
}

func (m grpcExactExecutionCommandManager) CancelPendingTaskTransition(ctx context.Context, in ExactPendingTransitionCommand) (*CommandResult, error) {
	resp, err := m.client.CancelPendingTaskTransitionExact(ctx, &pluginv1.CancelPendingTaskTransitionExactRequest{
		RequestId: in.RequestID, WorkspaceId: in.WorkspaceID, TaskId: in.TaskID,
		TransitionId: in.TransitionID, ExpectedResourceVersion: in.ExpectedResourceVersion,
		IdempotencyKey: in.IdempotencyKey, ApprovalRevision: in.ApprovalRevision, ManifestDigest: in.ManifestDigest,
		ManagementInstanceKey: in.ManagementInstanceKey, ExpectedClaimGeneration: in.ExpectedClaimGeneration,
	})
	if err != nil {
		return nil, err
	}
	return commandResultFromProto(resp.GetResult()), nil
}

func (m grpcExactExecutionCommandManager) GetSessionModeContext(ctx context.Context, in ExactTaskExecutionCommand) (SessionModeContext, error) {
	resp, err := m.client.GetSessionModeContextExact(ctx, &pluginv1.GetSessionModeContextExactRequest{
		RequestId: in.RequestID, WorkspaceId: in.WorkspaceID, TaskId: in.TaskID, SessionId: in.SessionID,
		ApprovalRevision: in.ApprovalRevision, ManifestDigest: in.ManifestDigest,
		ManagementInstanceKey: in.ManagementInstanceKey, ExpectedClaimGeneration: in.ExpectedClaimGeneration,
	})
	if err != nil {
		return SessionModeContext{}, err
	}
	return sessionModeContextFromProto(resp), nil
}

func (m grpcExactExecutionCommandManager) SetSessionMode(ctx context.Context, in ExactSessionModeCommand) (*CommandResult, string, error) {
	resp, err := m.client.SetSessionModeExact(ctx, &pluginv1.SetSessionModeExactRequest{
		RequestId: in.RequestID, WorkspaceId: in.WorkspaceID, TaskId: in.TaskID, SessionId: in.SessionID,
		ExpectedSessionResourceVersion: in.ExpectedSessionResourceVersion, ExpectedExecutionId: in.ExpectedExecutionID,
		ModeId: in.ModeID, IdempotencyKey: in.IdempotencyKey, ApprovalRevision: in.ApprovalRevision,
		ManifestDigest: in.ManifestDigest, ManagementInstanceKey: in.ManagementInstanceKey,
		ExpectedClaimGeneration: in.ExpectedClaimGeneration,
	})
	if err != nil {
		return nil, "", err
	}
	return commandResultFromProto(resp.GetResult()), resp.GetCurrentModeId(), nil
}

func (m grpcExactInteractionCommandManager) RespondPermission(ctx context.Context, in ExactPermissionResponse) (*CommandResult, *Interaction, error) {
	resp, err := m.client.RespondPermissionExact(ctx, &pluginv1.RespondPermissionExactRequest{
		RequestId: in.RequestID, WorkspaceId: in.WorkspaceID, InteractionId: in.InteractionID,
		ExpectedResourceVersion: in.ExpectedResourceVersion, OptionId: in.OptionID, Cancelled: in.Cancelled,
		HumanResponseReceiptId: in.HumanResponseReceiptID, ApprovalRevision: in.ApprovalRevision,
		ManifestDigest: in.ManifestDigest,
	})
	if err != nil {
		return nil, nil, err
	}
	return commandResultFromProto(resp.GetResult()), interactionPtrFromProto(resp.GetInteraction()), nil
}

func (m grpcExactInteractionCommandManager) AnswerClarification(ctx context.Context, in ExactClarificationResponse) (*CommandResult, *Interaction, error) {
	resp, err := m.client.AnswerClarificationExact(ctx, &pluginv1.AnswerClarificationExactRequest{
		RequestId: in.RequestID, WorkspaceId: in.WorkspaceID, InteractionId: in.InteractionID,
		ExpectedResourceVersion: in.ExpectedResourceVersion, Answers: clarificationAnswersToProto(in.Answers),
		HumanResponseReceiptId: in.HumanResponseReceiptID, ApprovalRevision: in.ApprovalRevision,
		ManifestDigest: in.ManifestDigest,
	})
	if err != nil {
		return nil, nil, err
	}
	return commandResultFromProto(resp.GetResult()), interactionPtrFromProto(resp.GetInteraction()), nil
}

func (s *grpcHostServer) executionManager() (ExactExecutionCommandManager, error) {
	if s.executionCommands == nil {
		return nil, status.Error(codes.Unimplemented, "exact execution controls are unavailable")
	}
	return s.executionCommands.ExecutionCommands(), nil
}

func (s *grpcHostServer) interactionManager() (ExactInteractionCommandManager, error) {
	if s.interactionCommands == nil {
		return nil, status.Error(codes.Unimplemented, "exact interaction commands are unavailable")
	}
	return s.interactionCommands.ExactInteractionCommands(), nil
}

func (s *grpcHostServer) EnsureTaskRunExact(ctx context.Context, req *pluginv1.EnsureTaskRunExactRequest) (*pluginv1.EnsureTaskRunExactResponse, error) {
	m, err := s.executionManager()
	if err != nil {
		return nil, err
	}
	result, run, err := m.EnsureTaskRun(ctx, ExactTaskRunCommand{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(), IdempotencyKey: req.GetIdempotencyKey(), ExpectedTaskResourceVersion: req.GetExpectedTaskResourceVersion(), ManagementInstanceKey: req.GetManagementInstanceKey(), ExpectedClaimGeneration: req.GetExpectedClaimGeneration(), ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest()})
	if err != nil {
		return nil, err
	}
	return &pluginv1.EnsureTaskRunExactResponse{Result: commandResultToProto(result), SessionId: run.SessionID, SessionState: run.SessionState, ExecutionId: run.ExecutionID}, nil
}

func (s *grpcHostServer) StopTaskRunExact(ctx context.Context, req *pluginv1.StopTaskRunExactRequest) (*pluginv1.StopTaskRunExactResponse, error) {
	m, err := s.executionManager()
	if err != nil {
		return nil, err
	}
	result, stopped, err := m.StopTaskRun(ctx, ExactTaskExecutionCommand{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(), SessionID: req.GetSessionId(), ExpectedSessionResourceVersion: req.GetExpectedSessionResourceVersion(), ExpectedExecutionID: req.GetExpectedExecutionId(), IdempotencyKey: req.GetIdempotencyKey(), ManagementInstanceKey: req.GetManagementInstanceKey(), ExpectedClaimGeneration: req.GetExpectedClaimGeneration(), ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest()})
	if err != nil {
		return nil, err
	}
	return &pluginv1.StopTaskRunExactResponse{Result: commandResultToProto(result), Stopped: stopped}, nil
}

func (s *grpcHostServer) RecoverSessionExact(ctx context.Context, req *pluginv1.RecoverSessionExactRequest) (*pluginv1.RecoverSessionExactResponse, error) {
	m, err := s.executionManager()
	if err != nil {
		return nil, err
	}
	result, run, err := m.RecoverSession(ctx, ExactSessionRecoveryCommand{ExactTaskExecutionCommand: ExactTaskExecutionCommand{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(), SessionID: req.GetSessionId(), ExpectedSessionResourceVersion: req.GetExpectedSessionResourceVersion(), ExpectedExecutionID: req.GetExpectedExecutionId(), IdempotencyKey: req.GetIdempotencyKey(), ManagementInstanceKey: req.GetManagementInstanceKey(), ExpectedClaimGeneration: req.GetExpectedClaimGeneration(), ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest()}, Action: req.GetAction()})
	if err != nil {
		return nil, err
	}
	return &pluginv1.RecoverSessionExactResponse{Result: commandResultToProto(result), SessionId: run.SessionID, ExecutionId: run.ExecutionID}, nil
}

func (s *grpcHostServer) CancelPendingTaskTransitionExact(ctx context.Context, req *pluginv1.CancelPendingTaskTransitionExactRequest) (*pluginv1.CancelPendingTaskTransitionExactResponse, error) {
	m, err := s.executionManager()
	if err != nil {
		return nil, err
	}
	result, err := m.CancelPendingTaskTransition(ctx, ExactPendingTransitionCommand{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(), TransitionID: req.GetTransitionId(), ExpectedResourceVersion: req.GetExpectedResourceVersion(), IdempotencyKey: req.GetIdempotencyKey(), ManagementInstanceKey: req.GetManagementInstanceKey(), ExpectedClaimGeneration: req.GetExpectedClaimGeneration(), ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest()})
	if err != nil {
		return nil, err
	}
	return &pluginv1.CancelPendingTaskTransitionExactResponse{Result: commandResultToProto(result)}, nil
}

func (s *grpcHostServer) GetSessionModeContextExact(ctx context.Context, req *pluginv1.GetSessionModeContextExactRequest) (*pluginv1.GetSessionModeContextExactResponse, error) {
	m, err := s.executionManager()
	if err != nil {
		return nil, err
	}
	value, err := m.GetSessionModeContext(ctx, ExactTaskExecutionCommand{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(), SessionID: req.GetSessionId(), ManagementInstanceKey: req.GetManagementInstanceKey(), ExpectedClaimGeneration: req.GetExpectedClaimGeneration(), ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest()})
	if err != nil {
		return nil, err
	}
	return sessionModeContextToProto(value), nil
}

func (s *grpcHostServer) SetSessionModeExact(ctx context.Context, req *pluginv1.SetSessionModeExactRequest) (*pluginv1.SetSessionModeExactResponse, error) {
	m, err := s.executionManager()
	if err != nil {
		return nil, err
	}
	result, current, err := m.SetSessionMode(ctx, ExactSessionModeCommand{ExactTaskExecutionCommand: ExactTaskExecutionCommand{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(), SessionID: req.GetSessionId(), ExpectedSessionResourceVersion: req.GetExpectedSessionResourceVersion(), ExpectedExecutionID: req.GetExpectedExecutionId(), IdempotencyKey: req.GetIdempotencyKey(), ManagementInstanceKey: req.GetManagementInstanceKey(), ExpectedClaimGeneration: req.GetExpectedClaimGeneration(), ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest()}, ModeID: req.GetModeId()})
	if err != nil {
		return nil, err
	}
	return &pluginv1.SetSessionModeExactResponse{Result: commandResultToProto(result), CurrentModeId: current}, nil
}

func (s *grpcHostServer) RespondPermissionExact(ctx context.Context, req *pluginv1.RespondPermissionExactRequest) (*pluginv1.RespondPermissionExactResponse, error) {
	m, err := s.interactionManager()
	if err != nil {
		return nil, err
	}
	result, interaction, err := m.RespondPermission(ctx, ExactPermissionResponse{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), InteractionID: req.GetInteractionId(), ExpectedResourceVersion: req.GetExpectedResourceVersion(), OptionID: req.GetOptionId(), Cancelled: req.GetCancelled(), HumanResponseReceiptID: req.GetHumanResponseReceiptId(), ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest()})
	if err != nil {
		return nil, err
	}
	return &pluginv1.RespondPermissionExactResponse{Result: commandResultToProto(result), Interaction: interactionPtrToProto(interaction)}, nil
}

func (s *grpcHostServer) AnswerClarificationExact(ctx context.Context, req *pluginv1.AnswerClarificationExactRequest) (*pluginv1.AnswerClarificationExactResponse, error) {
	m, err := s.interactionManager()
	if err != nil {
		return nil, err
	}
	result, interaction, err := m.AnswerClarification(ctx, ExactClarificationResponse{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), InteractionID: req.GetInteractionId(), ExpectedResourceVersion: req.GetExpectedResourceVersion(), Answers: clarificationAnswersFromProto(req.GetAnswers()), HumanResponseReceiptID: req.GetHumanResponseReceiptId(), ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest()})
	if err != nil {
		return nil, err
	}
	return &pluginv1.AnswerClarificationExactResponse{Result: commandResultToProto(result), Interaction: interactionPtrToProto(interaction)}, nil
}

func stopTaskRunToProto(in ExactTaskExecutionCommand) *pluginv1.StopTaskRunExactRequest {
	return &pluginv1.StopTaskRunExactRequest{RequestId: in.RequestID, WorkspaceId: in.WorkspaceID, TaskId: in.TaskID, SessionId: in.SessionID, ExpectedSessionResourceVersion: in.ExpectedSessionResourceVersion, ExpectedExecutionId: in.ExpectedExecutionID, IdempotencyKey: in.IdempotencyKey, ManagementInstanceKey: in.ManagementInstanceKey, ExpectedClaimGeneration: in.ExpectedClaimGeneration, ApprovalRevision: in.ApprovalRevision, ManifestDigest: in.ManifestDigest}
}

func sessionModeContextToProto(in SessionModeContext) *pluginv1.GetSessionModeContextExactResponse {
	modes := make([]*pluginv1.SessionModeOption, len(in.AvailableModes))
	for i, v := range in.AvailableModes {
		modes[i] = &pluginv1.SessionModeOption{Id: v.ID, Name: v.Name, Description: v.Description}
	}
	return &pluginv1.GetSessionModeContextExactResponse{SessionId: in.SessionID, ExecutionId: in.ExecutionID, CurrentModeId: in.CurrentModeID, AvailableModes: modes, ResourceVersion: in.ResourceVersion, ObservedAt: in.ObservedAt}
}

func sessionModeContextFromProto(in *pluginv1.GetSessionModeContextExactResponse) SessionModeContext {
	if in == nil {
		return SessionModeContext{}
	}
	modes := make([]SessionModeOption, len(in.GetAvailableModes()))
	for i, v := range in.GetAvailableModes() {
		modes[i] = SessionModeOption{ID: v.GetId(), Name: v.GetName(), Description: v.GetDescription()}
	}
	return SessionModeContext{SessionID: in.GetSessionId(), ExecutionID: in.GetExecutionId(), CurrentModeID: in.GetCurrentModeId(), AvailableModes: modes, ResourceVersion: in.GetResourceVersion(), ObservedAt: in.GetObservedAt()}
}

func interactionPtrFromProto(in *pluginv1.Interaction) *Interaction {
	if in == nil {
		return nil
	}
	value := interactionFromProto(in)
	return &value
}

func interactionPtrToProto(in *Interaction) *pluginv1.Interaction {
	if in == nil {
		return nil
	}
	return in.toProto()
}

var _ ExactExecutionCommandManager = grpcExactExecutionCommandManager{}
var _ ExactInteractionCommandManager = grpcExactInteractionCommandManager{}
var _ pluginv1.HostServer = (*grpcHostServer)(nil)
