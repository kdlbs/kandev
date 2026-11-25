package pluginsdk

import (
	"context"

	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
)

func (m grpcExactExecutionCommandManager) GetStepApproval(ctx context.Context, in ExactStepApprovalQuery) (StepApproval, error) {
	resp, err := m.client.GetStepApprovalExact(ctx, &pluginv1.GetStepApprovalExactRequest{
		RequestId: in.RequestID, WorkspaceId: in.WorkspaceID, SessionId: in.SessionID,
	})
	if err != nil {
		return StepApproval{}, err
	}
	return StepApproval{
		SessionID: resp.GetSessionId(), TaskID: resp.GetTaskId(), Awaiting: resp.GetAwaiting(),
		FromStepID: resp.GetFromStepId(), FromStepName: resp.GetFromStepName(),
		ToStepID: resp.GetToStepId(), ToStepName: resp.GetToStepName(),
		ResourceVersion: resp.GetResourceVersion(), ObservedAt: resp.GetObservedAt(),
	}, nil
}

func (m grpcExactExecutionCommandManager) ApproveStep(ctx context.Context, in ExactStepApprovalCommand) (*CommandResult, StepApprovalResult, error) {
	resp, err := m.client.ApproveStepExact(ctx, &pluginv1.ApproveStepExactRequest{
		RequestId: in.RequestID, WorkspaceId: in.WorkspaceID, SessionId: in.SessionID,
		ExpectedResourceVersion: in.ExpectedResourceVersion, IdempotencyKey: in.IdempotencyKey,
		HumanResponseReceiptId: in.HumanResponseReceiptID, ApprovalRevision: in.ApprovalRevision,
		ManifestDigest: in.ManifestDigest,
	})
	if err != nil {
		return nil, StepApprovalResult{}, err
	}
	return commandResultFromProto(resp.GetResult()), StepApprovalResult{FromStepID: resp.GetFromStepId(), ToStepID: resp.GetToStepId()}, nil
}

func (s *grpcHostServer) GetStepApprovalExact(ctx context.Context, req *pluginv1.GetStepApprovalExactRequest) (*pluginv1.GetStepApprovalExactResponse, error) {
	m, err := s.executionManager()
	if err != nil {
		return nil, err
	}
	approval, err := m.GetStepApproval(ctx, ExactStepApprovalQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), SessionID: req.GetSessionId()})
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetStepApprovalExactResponse{
		SessionId: approval.SessionID, TaskId: approval.TaskID, Awaiting: approval.Awaiting,
		FromStepId: approval.FromStepID, FromStepName: approval.FromStepName,
		ToStepId: approval.ToStepID, ToStepName: approval.ToStepName,
		ResourceVersion: approval.ResourceVersion, ObservedAt: approval.ObservedAt,
	}, nil
}

func (s *grpcHostServer) ApproveStepExact(ctx context.Context, req *pluginv1.ApproveStepExactRequest) (*pluginv1.ApproveStepExactResponse, error) {
	m, err := s.executionManager()
	if err != nil {
		return nil, err
	}
	result, approved, err := m.ApproveStep(ctx, ExactStepApprovalCommand{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), SessionID: req.GetSessionId(),
		ExpectedResourceVersion: req.GetExpectedResourceVersion(), IdempotencyKey: req.GetIdempotencyKey(),
		HumanResponseReceiptID: req.GetHumanResponseReceiptId(), ApprovalRevision: req.GetApprovalRevision(),
		ManifestDigest: req.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.ApproveStepExactResponse{Result: commandResultToProto(result), FromStepId: approved.FromStepID, ToStepId: approved.ToStepID}, nil
}
