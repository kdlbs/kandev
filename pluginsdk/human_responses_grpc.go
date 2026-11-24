package pluginsdk

import (
	"context"

	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type grpcHumanResponseDelegationManager struct{ client pluginv1.HostClient }

func (h *grpcHostClient) HumanResponseDelegation() HumanResponseDelegationManager {
	return grpcHumanResponseDelegationManager{client: h.client}
}

func (m grpcHumanResponseDelegationManager) LinkExternalIdentity(ctx context.Context, in ExternalIdentityLinkRequest) (*CommandResult, ExternalIdentityLink, error) {
	resp, err := m.client.LinkExternalIdentity(ctx, &pluginv1.LinkExternalIdentityRequest{
		RequestId: in.RequestID, Code: in.Code, Provider: in.Provider, Subject: in.Subject, DisplayName: in.DisplayName,
	})
	if err != nil {
		return nil, ExternalIdentityLink{}, err
	}
	return commandResultFromProto(resp.GetResult()), externalIdentityLinkFromProto(resp.GetLink()), nil
}

func (m grpcHumanResponseDelegationManager) RequestHumanResponseReceipt(ctx context.Context, in HumanResponseReceiptRequest) (*CommandResult, HumanResponseReceipt, error) {
	resp, err := m.client.RequestHumanResponseReceipt(ctx, &pluginv1.RequestHumanResponseReceiptRequest{
		RequestId: in.RequestID, WorkspaceId: in.WorkspaceID, Provider: in.Provider, Subject: in.Subject,
		Kind: in.Kind, TargetId: in.TargetID, ExpectedResourceVersion: in.ExpectedResourceVersion,
		OptionId: in.OptionID, Cancelled: in.Cancelled, Answers: clarificationAnswersToProto(in.Answers),
		ApprovalRevision: in.ApprovalRevision, ManifestDigest: in.ManifestDigest,
	})
	if err != nil {
		return nil, HumanResponseReceipt{}, err
	}
	return commandResultFromProto(resp.GetResult()), humanResponseReceiptFromProto(resp.GetReceipt()), nil
}

func (s *grpcHostServer) humanResponseManager() (HumanResponseDelegationManager, error) {
	if s.humanResponses == nil || s.humanResponses.HumanResponseDelegation() == nil {
		return nil, status.Error(codes.Unimplemented, "linked human responses are unavailable")
	}
	return s.humanResponses.HumanResponseDelegation(), nil
}

func (s *grpcHostServer) LinkExternalIdentity(ctx context.Context, req *pluginv1.LinkExternalIdentityRequest) (*pluginv1.LinkExternalIdentityResponse, error) {
	m, err := s.humanResponseManager()
	if err != nil {
		return nil, err
	}
	result, link, err := m.LinkExternalIdentity(ctx, ExternalIdentityLinkRequest{
		RequestID: req.GetRequestId(), Code: req.GetCode(), Provider: req.GetProvider(),
		Subject: req.GetSubject(), DisplayName: req.GetDisplayName(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.LinkExternalIdentityResponse{Result: commandResultToProto(result), Link: externalIdentityLinkToProto(link)}, nil
}

func (s *grpcHostServer) RequestHumanResponseReceipt(ctx context.Context, req *pluginv1.RequestHumanResponseReceiptRequest) (*pluginv1.RequestHumanResponseReceiptResponse, error) {
	m, err := s.humanResponseManager()
	if err != nil {
		return nil, err
	}
	result, receipt, err := m.RequestHumanResponseReceipt(ctx, HumanResponseReceiptRequest{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), Provider: req.GetProvider(), Subject: req.GetSubject(),
		Kind: req.GetKind(), TargetID: req.GetTargetId(), ExpectedResourceVersion: req.GetExpectedResourceVersion(),
		OptionID: req.GetOptionId(), Cancelled: req.GetCancelled(), Answers: clarificationAnswersFromProto(req.GetAnswers()),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.RequestHumanResponseReceiptResponse{Result: commandResultToProto(result), Receipt: humanResponseReceiptToProto(receipt)}, nil
}

func externalIdentityLinkToProto(in ExternalIdentityLink) *pluginv1.ExternalIdentityLink {
	if in == (ExternalIdentityLink{}) {
		return nil
	}
	return &pluginv1.ExternalIdentityLink{
		Id: in.ID, Provider: in.Provider, Subject: in.Subject, DisplayName: in.DisplayName,
		UserId: in.UserID, UserDisplayName: in.UserDisplayName, CreatedAt: in.CreatedAt,
	}
}

func externalIdentityLinkFromProto(p *pluginv1.ExternalIdentityLink) ExternalIdentityLink {
	if p == nil {
		return ExternalIdentityLink{}
	}
	return ExternalIdentityLink{
		ID: p.GetId(), Provider: p.GetProvider(), Subject: p.GetSubject(), DisplayName: p.GetDisplayName(),
		UserID: p.GetUserId(), UserDisplayName: p.GetUserDisplayName(), CreatedAt: p.GetCreatedAt(),
	}
}

func humanResponseReceiptToProto(in HumanResponseReceipt) *pluginv1.HumanResponseReceipt {
	if in.ID == "" {
		return nil
	}
	actor := in.Actor
	return &pluginv1.HumanResponseReceipt{
		Id: in.ID, Kind: in.Kind, TargetId: in.TargetID, ResourceVersion: in.ResourceVersion,
		ExpiresAt: in.ExpiresAt, Actor: interactionActorToProto(&actor),
	}
}

func humanResponseReceiptFromProto(p *pluginv1.HumanResponseReceipt) HumanResponseReceipt {
	if p == nil {
		return HumanResponseReceipt{}
	}
	receipt := HumanResponseReceipt{
		ID: p.GetId(), Kind: p.GetKind(), TargetID: p.GetTargetId(),
		ResourceVersion: p.GetResourceVersion(), ExpiresAt: p.GetExpiresAt(),
	}
	if actor := interactionActorFromProto(p.GetActor()); actor != nil {
		receipt.Actor = *actor
	}
	return receipt
}

func interactionActorToProto(in *InteractionActor) *pluginv1.InteractionActor {
	if in == nil || *in == (InteractionActor{}) {
		return nil
	}
	return &pluginv1.InteractionActor{UserId: in.UserID, DisplayName: in.DisplayName, Via: in.Via}
}

func interactionActorFromProto(p *pluginv1.InteractionActor) *InteractionActor {
	if p == nil {
		return nil
	}
	return &InteractionActor{UserID: p.GetUserId(), DisplayName: p.GetDisplayName(), Via: p.GetVia()}
}
